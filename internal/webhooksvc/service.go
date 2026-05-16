package webhooksvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
)

type Recorder interface {
	RecordWebhookEvent(context.Context, domain.WebhookEvent) (string, error)
}

type Reconciler interface {
	Recorder
	GetPaymentIntentByExternalRef(context.Context, string) (domain.PaymentIntent, error)
	UpsertPaymentIntent(context.Context, domain.PaymentIntent) (string, error)
}

type RefundReconciler interface {
	UpdateRefundStatusByProviderIdentifiers(context.Context, domain.ProviderCode, []string, domain.PaymentStatus, json.RawMessage) (domain.Refund, error)
	UpdatePaymentIntentStatusByID(context.Context, string, domain.PaymentStatus) error
}

type WebhookEventLookup interface {
	GetWebhookEventByProviderEventID(context.Context, domain.ProviderCode, string) (domain.WebhookEvent, error)
}

type Forwarder interface {
	Forward(context.Context, forwarding.InboundWebhook) error
}

type Service struct {
	recorder  Recorder
	forwarder Forwarder
	handlers  map[domain.ProviderCode]provider.Adapter
	now       func() time.Time
}

type Input struct {
	Provider domain.ProviderCode
	Request  provider.WebhookRequest
}

type Result struct {
	StatusCode int
	Body       map[string]any
	EventID    string
}

func New(recorder Recorder, forwarder Forwarder, handlers map[domain.ProviderCode]provider.Adapter) *Service {
	if handlers == nil {
		handlers = make(map[domain.ProviderCode]provider.Adapter)
	}
	return &Service{
		recorder:  recorder,
		forwarder: forwarder,
		handlers:  handlers,
		now:       time.Now,
	}
}

func (s *Service) Process(ctx context.Context, input Input) (Result, error) {
	if s == nil {
		return Result{StatusCode: http.StatusInternalServerError, Body: map[string]any{"error": "webhook service is not configured"}}, fmt.Errorf("webhook service is not configured")
	}

	providerCode := domain.ProviderCode(strings.TrimSpace(string(input.Provider)))
	if providerCode == "" {
		return Result{StatusCode: http.StatusBadRequest, Body: map[string]any{"error": "provider is required"}}, nil
	}

	request := provider.WebhookRequest{
		Headers: cloneHeaders(input.Request.Headers),
		Body:    cloneBytes(input.Request.Body),
	}
	event := domain.WebhookEvent{
		ProviderCode:     providerCode,
		EventType:        "unknown",
		SignatureValid:   false,
		PayloadJSON:      json.RawMessage(request.Body),
		HeadersJSON:      marshalHeaders(request.Headers),
		ReceivedAt:       s.now().UTC(),
		ProcessingStatus: "received",
	}

	handler, handlerFound := s.handlers[providerCode]
	if handlerFound {
		if err := handler.VerifyWebhook(ctx, request); err != nil {
			event.EventType = "verification_failed"
			event.ProcessingStatus = "rejected"
			_, _ = s.recordEvent(ctx, event)
			return Result{
				StatusCode: http.StatusBadRequest,
				Body: map[string]any{
					"status": "rejected",
					"error":  err.Error(),
				},
			}, nil
		}

		event.SignatureValid = true
		parsedEvent, parseErr := handler.ParseWebhook(ctx, request)
		if parseErr != nil {
			event.EventType = "parse_error"
			event.ProcessingStatus = "parse_failed"

			eventID, recordErr := s.recordEvent(ctx, event)
			if recordErr != nil {
				return Result{
					StatusCode: http.StatusInternalServerError,
					Body: map[string]any{
						"error": "failed to record webhook event",
					},
				}, recordErr
			}

			return Result{
				StatusCode: http.StatusAccepted,
				Body: map[string]any{
					"status":  "accepted",
					"warning": "webhook verification passed but parsing failed",
				},
				EventID: eventID,
			}, nil
		}

		event.EventType = parsedEvent.EventType
		event.ProviderEventID = parsedEvent.ProviderEventID

		duplicate, err := s.isDuplicateWebhookEvent(ctx, providerCode, parsedEvent.ProviderEventID)
		if err != nil {
			event.ProcessingStatus = "duplicate_lookup_failed"
			eventID, recordErr := s.recordEvent(ctx, event)
			if recordErr != nil {
				return Result{
					StatusCode: http.StatusInternalServerError,
					Body: map[string]any{
						"error": "failed to record webhook event",
					},
				}, recordErr
			}

			return Result{
				StatusCode: http.StatusInternalServerError,
				Body: map[string]any{
					"status":  "error",
					"error":   err.Error(),
					"message": "webhook duplicate lookup failed",
				},
				EventID: eventID,
			}, err
		}
		if duplicate {
			event.ProcessingStatus = "duplicate"
			eventID, recordErr := s.recordEvent(ctx, event)
			if recordErr != nil {
				return Result{
					StatusCode: http.StatusInternalServerError,
					Body: map[string]any{
						"error": "failed to record webhook event",
					},
				}, recordErr
			}

			return Result{
				StatusCode: http.StatusAccepted,
				Body: map[string]any{
					"status":  "accepted",
					"warning": "duplicate webhook event ignored",
				},
				EventID: eventID,
			}, nil
		}

		processingStatus, err := s.reconcilePaymentIntent(ctx, providerCode, parsedEvent)
		event.ProcessingStatus = processingStatus
		if err != nil {
			event.ProcessingStatus = "reconcile_failed"

			eventID, recordErr := s.recordEvent(ctx, event)
			if recordErr != nil {
				return Result{
					StatusCode: http.StatusInternalServerError,
					Body: map[string]any{
						"error": "failed to record webhook event",
					},
				}, recordErr
			}

			return Result{
				StatusCode: http.StatusInternalServerError,
				Body: map[string]any{
					"status":  "error",
					"error":   err.Error(),
					"message": "webhook reconciliation failed",
				},
				EventID: eventID,
			}, err
		}
	}

	eventID, err := s.recordEvent(ctx, event)
	if err != nil {
		return Result{
			StatusCode: http.StatusInternalServerError,
			Body: map[string]any{
				"error": "failed to record webhook event",
			},
		}, err
	}

	inbound := forwarding.InboundWebhook{
		WebhookEventID: eventID,
		Provider:       providerCode,
		Headers:        request.Headers.Clone(),
		Body:           request.Body,
	}
	if s.forwarder != nil {
		if err := s.forwarder.Forward(ctx, inbound); err != nil {
			return Result{
				StatusCode: http.StatusAccepted,
				Body: map[string]any{
					"status":  "accepted",
					"warning": fmt.Sprintf("forwarding failed: %v", err),
				},
				EventID: eventID,
			}, nil
		}
	}

	return Result{
		StatusCode: http.StatusAccepted,
		Body: map[string]any{
			"status": "accepted",
		},
		EventID: eventID,
	}, nil
}

func (s *Service) recordEvent(ctx context.Context, event domain.WebhookEvent) (string, error) {
	if s.recorder == nil {
		return "", nil
	}

	if event.ProcessedAt == nil {
		processedAt := s.now().UTC()
		event.ProcessedAt = &processedAt
	}

	id, err := s.recorder.RecordWebhookEvent(ctx, event)
	if err != nil {
		return "", fmt.Errorf("record webhook event: %w", err)
	}
	return id, nil
}

func (s *Service) isDuplicateWebhookEvent(ctx context.Context, providerCode domain.ProviderCode, providerEventID string) (bool, error) {
	if s == nil || s.recorder == nil {
		return false, nil
	}

	providerEventID = strings.TrimSpace(providerEventID)
	if providerEventID == "" {
		return false, nil
	}

	lookup, ok := s.recorder.(WebhookEventLookup)
	if !ok {
		return false, nil
	}

	_, err := lookup.GetWebhookEventByProviderEventID(ctx, providerCode, providerEventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("lookup webhook event by provider event id: %w", err)
	}

	return true, nil
}

func (s *Service) reconcilePaymentIntent(ctx context.Context, providerCode domain.ProviderCode, parsedEvent provider.WebhookEvent) (string, error) {
	if isRefundWebhook(parsedEvent) {
		if status, err := s.reconcileRefund(ctx, providerCode, parsedEvent); status != "" || err != nil {
			return status, err
		}
	}

	reconciler, ok := s.recorder.(Reconciler)
	if !ok {
		return "processed", nil
	}

	reference := strings.TrimSpace(parsedEvent.PaymentRef)
	if reference == "" {
		return "unmatched", nil
	}

	intent, err := reconciler.GetPaymentIntentByExternalRef(ctx, reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "unmatched", nil
		}
		return "reconcile_failed", err
	}

	if intent.ProviderCode != providerCode {
		return "unmatched", nil
	}

	if intent.Status == parsedEvent.Status {
		return "duplicate", nil
	}

	intent.Status = parsedEvent.Status
	_, err = reconciler.UpsertPaymentIntent(ctx, intent)
	if err != nil {
		return "reconcile_failed", err
	}

	return "reconciled", nil
}

func (s *Service) reconcileRefund(ctx context.Context, providerCode domain.ProviderCode, parsedEvent provider.WebhookEvent) (string, error) {
	reconciler, ok := s.recorder.(RefundReconciler)
	if !ok {
		return "", nil
	}

	status := parsedEvent.Status
	if status == "" {
		return "unmatched", nil
	}

	identifiers := refundWebhookIdentifiers(parsedEvent)
	if len(identifiers) == 0 {
		return "unmatched", nil
	}

	refund, err := reconciler.UpdateRefundStatusByProviderIdentifiers(ctx, providerCode, identifiers, status, parsedEvent.RawPayloadJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "unmatched", nil
		}
		return "reconcile_failed", err
	}

	if isRefundStatus(status) {
		if err := reconciler.UpdatePaymentIntentStatusByID(ctx, refund.PaymentIntentID, status); err != nil {
			return "reconcile_failed", err
		}
	}

	return "reconciled", nil
}

func isRefundWebhook(event provider.WebhookEvent) bool {
	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	return strings.HasPrefix(eventType, "refund.") || strings.HasSuffix(eventType, ".refund")
}

func isRefundStatus(status domain.PaymentStatus) bool {
	return status == domain.PaymentStatusRefunded || status == domain.PaymentStatusPartialRefunded
}

func refundWebhookIdentifiers(event provider.WebhookEvent) []string {
	candidates := []string{event.PaymentRef, event.ProviderEventID}
	if _, value, ok := strings.Cut(event.ProviderEventID, ":"); ok {
		candidates = append(candidates, value)
	}

	seen := make(map[string]struct{}, len(candidates))
	identifiers := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		identifiers = append(identifiers, trimmed)
	}
	return identifiers
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	return headers.Clone()
}

func cloneBytes(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	return append([]byte(nil), body...)
}

func marshalHeaders(headers http.Header) json.RawMessage {
	raw, err := json.Marshal(headers)
	if err != nil {
		return nil
	}
	return raw
}
