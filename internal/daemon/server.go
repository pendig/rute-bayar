package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
)

type Server struct {
	addr             string
	webhookRecorder  WebhookRecorder
	forwarder        *forwarding.Service
	providerHandlers map[domain.ProviderCode]provider.Adapter
}

type WebhookRecorder interface {
	RecordWebhookEvent(context.Context, domain.WebhookEvent) (string, error)
}

type webhookReconciler interface {
	GetPaymentIntentByExternalRef(context.Context, string) (domain.PaymentIntent, error)
	UpsertPaymentIntent(context.Context, domain.PaymentIntent) (string, error)
}

func NewServer(addr string, recorder WebhookRecorder, forwarder *forwarding.Service, handlers map[domain.ProviderCode]provider.Adapter) *Server {
	if addr == "" {
		addr = ":8080"
	}
	if handlers == nil {
		handlers = make(map[domain.ProviderCode]provider.Adapter)
	}
	return &Server{addr: addr, webhookRecorder: recorder, forwarder: forwarder, providerHandlers: handlers}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /webhooks/{provider}", s.webhook)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	providerCode := domain.ProviderCode(strings.TrimSpace(r.PathValue("provider")))
	if providerCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read webhook body"})
		return
	}

	headersJSON, _ := json.Marshal(r.Header)
	request := provider.WebhookRequest{
		Headers: r.Header,
		Body:    body,
	}
	webhookEventID := ""
	event := domain.WebhookEvent{
		ProviderCode:     providerCode,
		EventType:        "unknown",
		SignatureValid:   false,
		PayloadJSON:      body,
		HeadersJSON:      headersJSON,
		ReceivedAt:       time.Now().UTC(),
		ProcessingStatus: "received",
	}

	handler, handlerFound := s.providerHandlers[providerCode]
	if handlerFound {
		if err := handler.VerifyWebhook(r.Context(), request); err != nil {
			event.ProcessingStatus = "rejected"
			event.EventType = "verification_failed"
			if s.webhookRecorder != nil {
				id, recErr := s.webhookRecorder.RecordWebhookEvent(r.Context(), event)
				if recErr == nil {
					webhookEventID = id
				}
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "rejected",
				"error":  err.Error(),
			})
			return
		}

		event.SignatureValid = true
		parsedEvent, parseErr := handler.ParseWebhook(r.Context(), request)
		if parseErr != nil {
			event.EventType = "parse_error"
			event.ProcessingStatus = "parse_failed"
		} else {
			event.EventType = parsedEvent.EventType
			event.ProviderEventID = parsedEvent.ProviderEventID
			event.ProcessingStatus, err = s.reconcilePaymentIntent(r.Context(), providerCode, parsedEvent)
			if err != nil {
				event.ProcessingStatus = "reconcile_failed"
				if s.webhookRecorder != nil {
					if id, recErr := s.webhookRecorder.RecordWebhookEvent(r.Context(), event); recErr == nil {
						webhookEventID = id
					}
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"status":  "error",
					"error":   err.Error(),
					"message": "webhook reconciliation failed",
				})
				return
			}
		}
	}

	if s.webhookRecorder != nil {
		id, err := s.webhookRecorder.RecordWebhookEvent(r.Context(), event)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record webhook event"})
			return
		}
		webhookEventID = id
	}

	if handlerFound && event.ProcessingStatus == "parse_failed" {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "accepted",
			"warning": "webhook verification passed but parsing failed",
		})
		return
	}

	inbound := forwarding.InboundWebhook{
		WebhookEventID: webhookEventID,
		Provider:       providerCode,
		Headers:        r.Header.Clone(),
		Body:           body,
	}

	if s.forwarder != nil {
		if err := s.forwarder.Forward(r.Context(), inbound); err != nil {
			writeJSON(w, http.StatusAccepted, map[string]string{
				"status":  "accepted",
				"warning": fmt.Sprintf("forwarding failed: %v", err),
			})
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) reconcilePaymentIntent(ctx context.Context, providerCode domain.ProviderCode, parsedEvent provider.WebhookEvent) (string, error) {
	reconciler, ok := s.webhookRecorder.(webhookReconciler)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
