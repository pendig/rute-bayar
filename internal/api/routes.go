package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/forwardingsvc"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
)

const errNotFound = "not_found"

type providerAccountRequest struct {
	Provider    string          `json:"provider"`
	Environment string          `json:"environment"`
	DisplayName string          `json:"display_name"`
	Credentials json.RawMessage `json:"credentials"`
	Config      json.RawMessage `json:"config"`
}

type paymentRequest struct {
	Provider        string `json:"provider"`
	Environment     string `json:"environment"`
	BaseURL         string `json:"base_url"`
	Reference       string `json:"reference"`
	ExternalRef     string `json:"external_ref"`
	ProviderRef     string `json:"provider_reference"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Method          string `json:"method"`
	Channel         string `json:"channel"`
	CustomerName    string `json:"customer_name"`
	CustomerEmail   string `json:"customer_email"`
	CustomerPhone   string `json:"customer_phone"`
	CardToken       string `json:"card_token"`
	NotificationURL string `json:"notification_url"`
	RefundReference string `json:"refund_reference"`
	Reason          string `json:"reason"`
}

type forwardingTargetRequest struct {
	Provider       string              `json:"provider"`
	Name           string              `json:"name"`
	URL            string              `json:"url"`
	Enabled        *bool               `json:"enabled"`
	Headers        map[string][]string `json:"headers"`
	EventFilter    map[string]string   `json:"event_filter"`
	RetryPolicy    retryPolicyRequest  `json:"retry_policy"`
	RetryPolicySet bool                `json:"-"`
}

type retryPolicyRequest struct {
	MaxAttempts int    `json:"max_attempts"`
	Timeout     string `json:"timeout"`
	Backoff     string `json:"backoff"`
}

func (s *Server) listProviderAccounts(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	provider, err := parseOptionalProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	environment, err := s.parseOptionalEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		return nil, err
	}
	accounts, err := store.ListProviderAccountsByFilter(r.Context(), provider, environment)
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(accounts))
	for _, account := range accounts {
		data = append(data, providerAccountPayload(account))
	}
	return data, nil
}

func (s *Server) createProviderAccount(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	var input providerAccountRequest
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	provider, err := parseProvider(input.Provider)
	if err != nil {
		return nil, err
	}
	environment, err := s.parseEnvironment(input.Environment)
	if err != nil {
		return nil, err
	}
	id, err := store.UpsertProviderAccount(r.Context(), domain.ProviderAccount{
		ProviderCode:   provider,
		Environment:    environment,
		DisplayName:    strings.TrimSpace(input.DisplayName),
		CredentialJSON: defaultRawJSON(input.Credentials),
		ConfigJSON:     defaultRawJSON(input.Config),
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func (s *Server) updateProviderAccount(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "provider account id is required")
	}
	var input providerAccountRequest
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	provider, err := parseProvider(input.Provider)
	if err != nil {
		return nil, err
	}
	environment, err := s.parseEnvironment(input.Environment)
	if err != nil {
		return nil, err
	}
	savedID, err := store.UpdateProviderAccountByID(r.Context(), id, domain.ProviderAccount{
		ProviderCode:   provider,
		Environment:    environment,
		DisplayName:    strings.TrimSpace(input.DisplayName),
		CredentialJSON: defaultRawJSON(input.Credentials),
		ConfigJSON:     defaultRawJSON(input.Config),
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return map[string]string{"id": savedID}, nil
}

func (s *Server) deleteProviderAccount(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	if err := store.DeleteProviderAccountByID(r.Context(), strings.TrimSpace(r.PathValue("id"))); err != nil {
		return nil, mapStoreError(err)
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *Server) listPayments(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	provider, err := parseOptionalProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	status := domain.PaymentStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	limit, offset := pagination(r)
	payments, err := store.ListPaymentIntents(r.Context(), provider, status, limit, offset)
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(payments))
	for _, payment := range payments {
		data = append(data, paymentPayload(payment))
	}
	return data, nil
}

func (s *Server) createPayment(r *http.Request) (any, error) {
	return s.withIdempotency(r, "payment:create", func() (any, error) {
		svc, err := s.requirePaymentService()
		if err != nil {
			return nil, err
		}
		var input paymentRequest
		if err := decodeBody(r, &input); err != nil {
			return nil, err
		}
		provider, err := parseProvider(input.Provider)
		if err != nil {
			return nil, err
		}
		env, err := s.parseEnvironment(input.Environment)
		if err != nil {
			return nil, err
		}
		ref := firstNonEmpty(input.ExternalRef, input.Reference)
		result, err := svc.Create(r.Context(), paymentsvc.CreateInput{
			Provider:        provider,
			Environment:     env,
			BaseURL:         input.BaseURL,
			ExternalRef:     ref,
			Amount:          input.Amount,
			Currency:        input.Currency,
			Method:          input.Method,
			Channel:         input.Channel,
			CustomerName:    input.CustomerName,
			CustomerEmail:   input.CustomerEmail,
			CustomerPhone:   input.CustomerPhone,
			CardToken:       input.CardToken,
			NotificationURL: input.NotificationURL,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (s *Server) getPayment(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	intent, err := store.GetPaymentIntentByExternalRef(r.Context(), strings.TrimSpace(r.PathValue("reference")))
	if err != nil {
		return nil, mapStoreError(err)
	}
	payload := paymentPayload(intent)
	if attempt, err := store.GetLatestPaymentAttemptByIntent(r.Context(), intent.ID, intent.ProviderCode); err == nil {
		payload["latest_attempt"] = paymentAttemptPayload(attempt)
	}
	return payload, nil
}

func (s *Server) getPaymentStatus(r *http.Request) (any, error) {
	svc, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}
	provider, err := parseProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	env, err := s.parseOptionalEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		return nil, err
	}
	result, err := svc.Status(r.Context(), paymentsvc.StatusInput{
		Provider:          provider,
		Environment:       env,
		BaseURL:           r.URL.Query().Get("base_url"),
		Reference:         strings.TrimSpace(r.PathValue("reference")),
		ProviderReference: r.URL.Query().Get("provider_reference"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) refundPayment(r *http.Request) (any, error) {
	return s.withIdempotency(r, "payment:refund", func() (any, error) {
		svc, err := s.requirePaymentService()
		if err != nil {
			return nil, err
		}
		var input paymentRequest
		if err := decodeBody(r, &input); err != nil {
			return nil, err
		}
		provider, err := parseProvider(input.Provider)
		if err != nil {
			return nil, err
		}
		env, err := s.parseEnvironment(input.Environment)
		if err != nil {
			return nil, err
		}
		result, err := svc.Refund(r.Context(), paymentsvc.RefundInput{
			Provider:          provider,
			Environment:       env,
			BaseURL:           input.BaseURL,
			Reference:         strings.TrimSpace(r.PathValue("reference")),
			ProviderReference: input.ProviderRef,
			RefundReference:   input.RefundReference,
			Amount:            input.Amount,
			Currency:          input.Currency,
			Reason:            input.Reason,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (s *Server) listWebhookEvents(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	provider, err := parseOptionalProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	signatureValid, err := optionalBool(r.URL.Query().Get("signature_valid"))
	if err != nil {
		return nil, err
	}
	limit, offset := pagination(r)
	events, err := store.ListWebhookEvents(r.Context(), provider, r.URL.Query().Get("status"), signatureValid, limit, offset)
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(events))
	for _, event := range events {
		data = append(data, webhookEventPayload(event))
	}
	return data, nil
}

func (s *Server) getWebhookEvent(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	event, err := store.GetWebhookEventByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		return nil, mapStoreError(err)
	}
	return webhookEventPayload(event), nil
}

func (s *Server) listWebhookEventAttempts(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	return listAttempts(r.Context(), store, forwarding.AttemptFilter{WebhookEventID: strings.TrimSpace(r.PathValue("id")), Limit: queryInt(r, "limit", 50)})
}

func (s *Server) replayWebhookEvent(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	event, err := store.GetWebhookEventByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		return nil, mapStoreError(err)
	}
	headers := http.Header{}
	_ = json.Unmarshal(event.HeadersJSON, &headers)
	if err := forwarding.NewService(store).Forward(r.Context(), forwarding.InboundWebhook{
		WebhookEventID: event.ID,
		Provider:       event.ProviderCode,
		Headers:        headers,
		Body:           event.PayloadJSON,
	}); err != nil {
		return nil, err
	}
	return map[string]string{"status": "replayed", "webhook_event_id": event.ID}, nil
}

func (s *Server) listForwardingTargets(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	provider, err := parseOptionalProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	targets, err := forwardingsvc.New(store).List(r.Context(), forwardingsvc.ListInput{
		Provider:        provider,
		IncludeDisabled: strings.EqualFold(r.URL.Query().Get("include_disabled"), "true"),
	})
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(targets))
	for _, target := range targets {
		data = append(data, forwardingTargetPayload(target))
	}
	return data, nil
}

func (s *Server) createForwardingTarget(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	input, err := decodeForwardingTarget(r)
	if err != nil {
		return nil, err
	}
	provider, err := parseProvider(input.Provider)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	id, err := forwardingsvc.New(store).Add(r.Context(), forwardingsvc.AddInput{
		Provider:    provider,
		Name:        input.Name,
		URL:         input.URL,
		Enabled:     enabled,
		Headers:     http.Header(input.Headers),
		EventFilter: input.EventFilter,
		RetryPolicy: retryPolicy(input.RetryPolicy),
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func (s *Server) updateForwardingTarget(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	input, err := decodeForwardingTarget(r)
	if err != nil {
		return nil, err
	}
	enabled := false
	enabledSet := input.Enabled != nil
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	err = forwardingsvc.New(store).Update(r.Context(), forwardingsvc.UpdateInput{
		ID:             strings.TrimSpace(r.PathValue("id")),
		Name:           input.Name,
		URL:            input.URL,
		Enabled:        enabled,
		EnabledSet:     enabledSet,
		Headers:        http.Header(input.Headers),
		HeadersSet:     input.Headers != nil,
		EventFilter:    input.EventFilter,
		EventFilterSet: input.EventFilter != nil,
		RetryPolicy:    retryPolicy(input.RetryPolicy),
		RetryPolicySet: input.RetryPolicySet,
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return map[string]string{"status": "updated"}, nil
}

func (s *Server) deleteForwardingTarget(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	if err := forwardingsvc.New(store).Remove(r.Context(), strings.TrimSpace(r.PathValue("id"))); err != nil {
		return nil, mapStoreError(err)
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *Server) listForwardingAttempts(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	provider, err := parseOptionalProvider(r.URL.Query().Get("provider"))
	if err != nil {
		return nil, err
	}
	return listAttempts(r.Context(), store, forwarding.AttemptFilter{
		Provider:       provider,
		TargetID:       r.URL.Query().Get("target_id"),
		WebhookEventID: r.URL.Query().Get("webhook_event_id"),
		Status:         r.URL.Query().Get("status"),
		Limit:          queryInt(r, "limit", 50),
	})
}

func (s *Server) reconcilePayment(r *http.Request) (any, error) {
	svc, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}
	var input paymentRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	rawProvider := firstNonEmpty(r.PathValue("provider"), input.Provider, r.URL.Query().Get("provider"))
	provider, err := parseProvider(rawProvider)
	if err != nil {
		return nil, err
	}
	env, err := s.parseOptionalEnvironment(firstNonEmpty(input.Environment, r.URL.Query().Get("environment")))
	if err != nil {
		return nil, err
	}
	result, err := svc.Reconcile(r.Context(), paymentsvc.ReconcileInput{
		Provider:          provider,
		Environment:       env,
		BaseURL:           firstNonEmpty(input.BaseURL, r.URL.Query().Get("base_url")),
		Reference:         firstNonEmpty(r.PathValue("reference"), input.Reference, input.ExternalRef),
		ProviderReference: firstNonEmpty(input.ProviderRef, r.URL.Query().Get("provider_reference")),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) stats(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	providerAccounts, err := store.CountProviderAccounts(r.Context())
	if err != nil {
		return nil, err
	}
	payments, err := store.CountPaymentIntents(r.Context())
	if err != nil {
		return nil, err
	}
	webhookEvents, err := store.CountWebhookEvents(r.Context())
	if err != nil {
		return nil, err
	}
	forwardingTargets, err := store.CountForwardingTargets(r.Context())
	if err != nil {
		return nil, err
	}
	forwardingAttempts, err := store.CountForwardingAttempts(r.Context())
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"provider_accounts":   providerAccounts,
		"payments":            payments,
		"webhook_events":      webhookEvents,
		"forwarding_targets":  forwardingTargets,
		"forwarding_attempts": forwardingAttempts,
	}, nil
}

func (s *Server) requireStore() (Store, error) {
	if s == nil || s.store == nil {
		return nil, NewError(http.StatusServiceUnavailable, errInternal, "api store is not configured")
	}
	return s.store, nil
}

func (s *Server) requirePaymentService() (PaymentService, error) {
	if s == nil || s.payments == nil {
		return nil, NewError(http.StatusServiceUnavailable, errInternal, "payment service is not configured")
	}
	return s.payments, nil
}

func defaultEnvironment(value domain.Environment) domain.Environment {
	if value == domain.EnvironmentProduction {
		return value
	}
	return domain.EnvironmentSandbox
}

func (s *Server) parseEnvironment(raw string) (domain.Environment, error) {
	if strings.TrimSpace(raw) == "" {
		return s.environment, nil
	}
	return parseEnvironment(raw)
}

func (s *Server) parseOptionalEnvironment(raw string) (domain.Environment, error) {
	if strings.TrimSpace(raw) == "" {
		return s.environment, nil
	}
	return parseEnvironment(raw)
}

func parseEnvironment(raw string) (domain.Environment, error) {
	switch domain.Environment(strings.ToLower(strings.TrimSpace(raw))) {
	case domain.EnvironmentSandbox:
		return domain.EnvironmentSandbox, nil
	case domain.EnvironmentProduction:
		return domain.EnvironmentProduction, nil
	default:
		return "", NewError(http.StatusBadRequest, errBadRequest, "environment must be sandbox or production")
	}
}

func parseProvider(raw string) (domain.ProviderCode, error) {
	provider := domain.ProviderCode(strings.ToLower(strings.TrimSpace(raw)))
	for _, supported := range domain.SupportedProviders() {
		if provider == supported {
			return supported, nil
		}
	}
	return "", NewError(http.StatusBadRequest, errBadRequest, "provider is required and must be supported")
}

func parseOptionalProvider(raw string) (domain.ProviderCode, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return parseProvider(raw)
}

func decodeBody(r *http.Request, target any) error {
	if r.Body == nil {
		return NewError(http.StatusBadRequest, errBadRequest, "request body is required")
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return NewError(http.StatusBadRequest, errBadRequest, "invalid json body")
	}
	return nil
}

func decodeForwardingTarget(r *http.Request) (forwardingTargetRequest, error) {
	var input forwardingTargetRequest
	if err := decodeBody(r, &input); err != nil {
		return input, err
	}
	var raw map[string]json.RawMessage
	if r.GetBody != nil {
		body, _ := r.GetBody()
		if body != nil {
			_ = json.NewDecoder(body).Decode(&raw)
			_ = body.Close()
		}
	}
	input.RetryPolicySet = input.RetryPolicy.MaxAttempts != 0 || input.RetryPolicy.Timeout != "" || input.RetryPolicy.Backoff != ""
	return input, nil
}

func defaultRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func pagination(r *http.Request) (int, int) {
	return queryInt(r, "limit", 50), queryInt(r, "offset", 0)
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	if key == "limit" && value > 500 {
		return 500
	}
	return value
}

func optionalBool(raw string) (*bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "boolean query value is invalid")
	}
	return &value, nil
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not configured") || strings.Contains(err.Error(), "not found") {
		return NewError(http.StatusNotFound, errNotFound, err.Error())
	}
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func retryPolicy(input retryPolicyRequest) forwarding.RetryPolicy {
	timeout, _ := time.ParseDuration(input.Timeout)
	backoff, _ := time.ParseDuration(input.Backoff)
	return forwarding.RetryPolicy{MaxAttempts: input.MaxAttempts, Timeout: timeout, Backoff: backoff}
}

func listAttempts(ctx context.Context, store Store, filter forwarding.AttemptFilter) (any, error) {
	attempts, err := store.ListForwardingAttempts(ctx, filter)
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(attempts))
	for _, attempt := range attempts {
		data = append(data, forwardingAttemptPayload(attempt))
	}
	return data, nil
}

func (s *Server) withIdempotency(r *http.Request, scope string, fn func() (any, error)) (any, error) {
	key := firstNonEmpty(r.Header.Get("Idempotency-Key"), r.Header.Get("X-Idempotency-Key"))
	if key == "" {
		return fn()
	}
	cacheKey := scope + ":" + key
	s.idemMu.Lock()
	if cached, ok := s.idempotency[cacheKey]; ok {
		s.idemMu.Unlock()
		return cached, nil
	}
	s.idemMu.Unlock()
	payload, err := fn()
	if err != nil {
		return nil, err
	}
	s.idemMu.Lock()
	s.idempotency[cacheKey] = payload
	s.idemMu.Unlock()
	return payload, nil
}

func providerAccountPayload(account domain.ProviderAccount) map[string]any {
	return map[string]any{
		"id":           account.ID,
		"provider":     account.ProviderCode,
		"environment":  account.Environment,
		"display_name": account.DisplayName,
		"credentials":  jsonObject(account.CredentialJSON),
		"config":       jsonObject(account.ConfigJSON),
		"created_at":   formatTime(account.CreatedAt),
		"updated_at":   formatTime(account.UpdatedAt),
	}
}

func paymentPayload(payment domain.PaymentIntent) map[string]any {
	return map[string]any{
		"id":         payment.ID,
		"reference":  payment.ExternalRef,
		"provider":   payment.ProviderCode,
		"amount":     payment.Amount,
		"currency":   payment.Currency,
		"status":     payment.Status,
		"metadata":   jsonObject(payment.MetadataJSON),
		"created_at": formatTime(payment.CreatedAt),
		"updated_at": formatTime(payment.UpdatedAt),
	}
}

func paymentAttemptPayload(attempt domain.PaymentAttempt) map[string]any {
	return map[string]any{
		"id":                 attempt.ID,
		"payment_intent_id":  attempt.PaymentIntentID,
		"provider":           attempt.ProviderCode,
		"status":             attempt.Status,
		"provider_reference": attempt.ProviderReference,
		"request":            jsonObject(attempt.RequestJSON),
		"response":           jsonObject(attempt.ResponseJSON),
		"created_at":         formatTime(attempt.CreatedAt),
		"updated_at":         formatTime(attempt.UpdatedAt),
	}
}

func webhookEventPayload(event domain.WebhookEvent) map[string]any {
	return map[string]any{
		"id":                event.ID,
		"provider":          event.ProviderCode,
		"provider_event_id": event.ProviderEventID,
		"event_type":        event.EventType,
		"signature_valid":   event.SignatureValid,
		"payload":           jsonObject(event.PayloadJSON),
		"headers":           jsonObject(event.HeadersJSON),
		"received_at":       formatTime(event.ReceivedAt),
		"processed_at":      formatOptionalTime(event.ProcessedAt),
		"processing_status": event.ProcessingStatus,
	}
}

func forwardingTargetPayload(target forwarding.Target) map[string]any {
	return map[string]any{
		"id":           target.ID,
		"provider":     target.Provider,
		"name":         target.Name,
		"url":          target.URL,
		"enabled":      target.Enabled,
		"headers":      target.Headers,
		"event_filter": target.EventFilter,
		"retry_policy": map[string]any{
			"max_attempts": target.RetryPolicy.MaxAttempts,
			"timeout":      target.RetryPolicy.Timeout.String(),
			"backoff":      target.RetryPolicy.Backoff.String(),
		},
	}
}

func forwardingAttemptPayload(attempt forwarding.AttemptRecord) map[string]any {
	return map[string]any{
		"id":               attempt.ID,
		"webhook_event_id": attempt.WebhookEventID,
		"target_id":        attempt.TargetID,
		"target_name":      attempt.TargetName,
		"target_url":       attempt.TargetURL,
		"provider":         attempt.Provider,
		"attempt_no":       attempt.AttemptNo,
		"request":          jsonObject(attempt.RequestJSON),
		"response":         jsonObject(attempt.ResponseJSON),
		"status":           attempt.Status,
		"created_at":       formatTime(attempt.CreatedAt),
		"updated_at":       formatTime(attempt.UpdatedAt),
	}
}

func jsonObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
