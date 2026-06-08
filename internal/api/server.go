package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pendig/rute-bayar/internal/auditlog"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/forwardingsvc"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
)

const (
	errUnauthorized = "unauthorized"
	errForbidden    = "forbidden"
	errRateLimited  = "rate_limited"
	errBadRequest   = "bad_request"
	errNotFound     = "not_found"
	errConflict     = "conflict"
	errInternal     = "internal_error"

	requestIDHeader = "X-Request-ID"
	requestIDQuery  = "request_id"
	headerOrigin    = "Origin"

	maxJSONBodyBytes = 1 * 1024 * 1024

	rateBucketLimit = 2000
	rateBucketTTL   = 10 * time.Minute
	idemTTL         = 10 * time.Minute
	idemLimit       = 2000
)

var requestIDSeq uint64

const (
	contentTypeJSON = "application/json"
)

// Server handles HTTP API routes used by daemon API mode.
type Server struct {
	version       string
	apiKey        string
	allowedOrigin string
	rateLimit     int
	auditStore    AuditStore
	store         Store
	payments      PaymentService
	environment   domain.Environment
	databasePath  string

	rateBuckets    map[string]*rateBucket
	bucketsMu      sync.Mutex
	idempotencyMu  sync.Mutex
	idempotencyMap map[string]idempotencyEntry
}

type Config struct {
	Version            string
	APIKey             string
	AllowedOrigins     string
	RateLimitPerMinute int
	AuditSink          AuditStore
	Store              Store
	PaymentService     PaymentService
	DatabasePath       string
	DefaultEnvironment domain.Environment
}

type Store interface {
	ListProviderAccounts(context.Context) ([]domain.ProviderAccount, error)
	GetProviderAccountByID(context.Context, string) (domain.ProviderAccount, error)
	UpsertProviderAccount(context.Context, domain.ProviderAccount) (string, error)
	UpdateProviderAccountByID(context.Context, domain.ProviderAccount) error
	DeleteProviderAccount(context.Context, string) error
	ListEnabledTargets(context.Context, domain.ProviderCode) ([]forwarding.Target, error)
	GetWebhookEventByID(context.Context, string) (domain.WebhookEvent, error)
	ListWebhookEvents(context.Context, domain.ProviderCode, string, *bool, int, int) ([]domain.WebhookEvent, error)
	CountWebhookEvents(context.Context, domain.ProviderCode, string, *bool) (int, error)
	GetPaymentIntentByExternalRef(context.Context, string) (domain.PaymentIntent, error)
	GetLatestPaymentAttemptByIntent(context.Context, string, domain.ProviderCode) (domain.PaymentAttempt, error)
	ListPaymentIntents(context.Context, domain.ProviderCode, domain.Environment, domain.PaymentStatus, int, int) ([]domain.PaymentIntent, error)
	CountPaymentIntents(context.Context, domain.ProviderCode, domain.Environment, domain.PaymentStatus) (int, error)
	ListForwardingTargets(context.Context, domain.ProviderCode) ([]forwarding.Target, error)
	AddForwardingTarget(context.Context, forwarding.Target) (string, error)
	GetForwardingTarget(context.Context, string) (forwarding.Target, error)
	UpdateForwardingTarget(context.Context, forwarding.Target) error
	DeleteForwardingTarget(context.Context, string) error
	ListForwardingAttempts(context.Context, forwarding.AttemptFilter) ([]forwarding.AttemptRecord, error)
	CountForwardingAttempts(context.Context, forwarding.AttemptFilter) (int, error)
	RecordAttempt(context.Context, forwarding.Attempt) error
	CountForwardingTargets(context.Context, domain.ProviderCode, bool) (int, error)
}

type PaymentService interface {
	Create(context.Context, paymentsvc.CreateInput) (paymentsvc.CreateResult, error)
	Status(context.Context, paymentsvc.StatusInput) (paymentsvc.StatusResult, error)
	Refund(context.Context, paymentsvc.RefundInput) (paymentsvc.RefundResult, error)
	Reconcile(context.Context, paymentsvc.ReconcileInput) (paymentsvc.ReconcileResult, error)
}

func NewServer(cfg Config) *Server {
	allowed := strings.TrimSpace(cfg.AllowedOrigins)
	if allowed == "" {
		allowed = "*"
	}
	rate := cfg.RateLimitPerMinute
	if rate <= 0 {
		rate = 120
	}

	return &Server{
		version:        strings.TrimSpace(cfg.Version),
		apiKey:         strings.TrimSpace(cfg.APIKey),
		allowedOrigin:  allowed,
		rateLimit:      rate,
		auditStore:     cfg.AuditSink,
		store:          cfg.Store,
		payments:       cfg.PaymentService,
		databasePath:   strings.TrimSpace(cfg.DatabasePath),
		environment:    defaultEnvironment(cfg.DefaultEnvironment),
		rateBuckets:    map[string]*rateBucket{},
		idempotencyMap: map[string]idempotencyEntry{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.wrap(s.health, false))
	mux.HandleFunc("GET /api/v1/healthz", s.wrap(s.health, false))
	mux.HandleFunc("GET /api/v1/version", s.wrap(s.versionHandler, true))
	mux.HandleFunc("POST /api/v1/version", s.wrap(s.versionHandler, true))

	mux.HandleFunc("GET /api/v1/provider-accounts", s.wrap(s.providerAccountsListHandler, true))
	mux.HandleFunc("POST /api/v1/provider-accounts", s.wrap(s.providerAccountsCreateHandler, true))
	mux.HandleFunc("PUT /api/v1/provider-accounts/{id}", s.wrap(s.providerAccountUpdateHandler, true))
	mux.HandleFunc("DELETE /api/v1/provider-accounts/{id}", s.wrap(s.providerAccountDeleteHandler, true))

	mux.HandleFunc("GET /api/v1/payments", s.wrap(s.paymentsListHandler, true))
	mux.HandleFunc("POST /api/v1/payments", s.wrap(s.paymentsCreateHandler, true))
	mux.HandleFunc("GET /api/v1/payments/{reference}", s.wrap(s.paymentGetHandler, true))
	mux.HandleFunc("GET /api/v1/payments/{reference}/status", s.wrap(s.paymentStatusHandler, true))
	mux.HandleFunc("POST /api/v1/payments/{reference}/refund", s.wrap(s.paymentRefundHandler, true))
	mux.HandleFunc("GET /api/v1/readiness", s.wrap(s.readinessHandler, false))

	mux.HandleFunc("GET /api/v1/webhook-events", s.wrap(s.webhookEventsListHandler, true))
	mux.HandleFunc("GET /api/v1/webhook-events/{id}", s.wrap(s.webhookEventGetHandler, true))
	mux.HandleFunc("GET /api/v1/webhook-events/{id}/forwarding-attempts", s.wrap(s.webhookEventForwardingAttemptsHandler, true))
	mux.HandleFunc("POST /api/v1/webhook-events/{id}/replay", s.wrap(s.webhookEventReplayHandler, true))
	mux.HandleFunc("GET /api/v1/webhook-forwarding-targets", s.wrap(s.webhookForwardingTargetsListHandler, true))
	mux.HandleFunc("POST /api/v1/webhook-forwarding-targets", s.wrap(s.webhookForwardingTargetCreateHandler, true))
	mux.HandleFunc("PUT /api/v1/webhook-forwarding-targets/{id}", s.wrap(s.webhookForwardingTargetUpdateHandler, true))
	mux.HandleFunc("DELETE /api/v1/webhook-forwarding-targets/{id}", s.wrap(s.webhookForwardingTargetDeleteHandler, true))
	mux.HandleFunc("GET /api/v1/webhook-forwarding-attempts", s.wrap(s.webhookForwardingAttemptsListHandler, true))
	mux.HandleFunc("POST /api/v1/reconcile/{provider}/{reference}", s.wrap(s.paymentReconcileHandler, true))
	mux.HandleFunc("GET /api/v1/stats", s.wrap(s.statsHandler, true))

	mux.Handle("/", http.NotFoundHandler())
	return mux
}

type endpointFunc func(*http.Request) (any, error)

func (s *Server) wrap(fn endpointFunc, requireAuth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}

		s.writeCORSHeaders(w)
		w.Header().Set(requestIDHeader, requestID)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if !s.allowRate(r, r.RemoteAddr) {
			s.writeJSONError(w, r, requestID, http.StatusTooManyRequests, errRateLimited, "rate limit exceeded")
			s.audit(r, requestID, http.StatusTooManyRequests, time.Since(start).Milliseconds())
			return
		}

		if requireAuth && !s.isAuthorized(r) {
			status := http.StatusUnauthorized
			code := errUnauthorized
			message := "missing or invalid API key"
			if s.apiKey == "" {
				status = http.StatusForbidden
				code = errForbidden
				message = "API key disabled"
			}
			s.writeJSONError(w, r, requestID, status, code, message)
			s.audit(r, requestID, status, time.Since(start).Milliseconds())
			return
		}

		payload, err := fn(r)
		if err != nil {
			if apiErr, ok := err.(*apiError); ok {
				s.writeJSONError(w, r, requestID, apiErr.Status, apiErr.Code, apiErr.Message)
				s.audit(r, requestID, apiErr.Status, time.Since(start).Milliseconds())
				return
			}
			s.writeJSONError(w, r, requestID, http.StatusBadRequest, errBadRequest, err.Error())
			s.audit(r, requestID, http.StatusBadRequest, time.Since(start).Milliseconds())
			return
		}

		s.writeJSON(w, r, requestID, http.StatusOK, payload)
		s.audit(r, requestID, http.StatusOK, time.Since(start).Milliseconds())
	}
}

func (s *Server) writeCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
	w.Header().Set("Vary", headerOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID, Idempotency-Key")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "3600")
}

func (s *Server) isAuthorized(r *http.Request) bool {
	if s.apiKey == "" {
		return false
	}
	provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
	return provided == s.apiKey && provided != ""
}

type rateBucket struct {
	start  time.Time
	count  int
	seenAt time.Time
}

func (s *Server) allowRate(r *http.Request, remoteAddr string) bool {
	if s.rateLimit <= 0 {
		return true
	}

	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if s.apiKey == "" || key != s.apiKey {
		key = remoteAddr
		if host, _, splitErr := net.SplitHostPort(remoteAddr); splitErr == nil {
			key = host
		}
	}
	if key == "" {
		key = "anonymous"
	}

	now := time.Now().UTC()
	window := now.Truncate(time.Minute)

	s.bucketsMu.Lock()
	defer s.bucketsMu.Unlock()
	if len(s.rateBuckets) >= rateBucketLimit {
		s.pruneRateBuckets(now)
	}
	if len(s.rateBuckets) >= rateBucketLimit {
		for bucketKey := range s.rateBuckets {
			delete(s.rateBuckets, bucketKey)
			if len(s.rateBuckets) < rateBucketLimit/2 {
				break
			}
		}
	}

	bucket, ok := s.rateBuckets[key]
	if !ok {
		bucket = &rateBucket{start: window, seenAt: now}
		s.rateBuckets[key] = bucket
	}

	if bucket.start != window {
		bucket.start = window
		bucket.count = 0
	}
	bucket.seenAt = now
	if bucket.count >= s.rateLimit {
		return false
	}
	bucket.count++
	return true
}

func (s *Server) pruneRateBuckets(now time.Time) {
	for key, bucket := range s.rateBuckets {
		if now.Sub(bucket.seenAt) > rateBucketTTL {
			delete(s.rateBuckets, key)
		}
	}
}

func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"error_code": code,
		"message":    message,
		"details": map[string]any{
			"path": r.URL.Path,
		},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, requestID string, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"data":       payload,
		"metadata": map[string]any{
			requestIDQuery: requestID,
		},
	})
}

func (s *Server) audit(r *http.Request, requestID string, status int, durationMs int64) {
	event := auditlog.Event{
		RequestID:  requestID,
		ActorType:  "api-client",
		ActorID:    strings.TrimSpace(r.Header.Get("X-API-Key")),
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     status,
		DurationMs: durationMs,
		ClientIP:   r.RemoteAddr,
	}
	if event.ActorID == "" {
		event.ActorID = "anonymous"
	}
	if s.auditStore == nil {
		log.Printf("api.request request_id=%s method=%s path=%s status=%d duration_ms=%d client_ip=%s", requestID, event.Method, event.Path, event.Status, event.DurationMs, event.ClientIP)
		return
	}
	_ = s.auditStore.RecordAuditEvent(r.Context(), event)
}

func (s *Server) health(r *http.Request) (any, error) {
	if strings.TrimSpace(r.Method) != http.MethodGet {
		return nil, NewError(http.StatusMethodNotAllowed, errBadRequest, "method not allowed")
	}
	return map[string]string{"status": "ok"}, nil
}

func (s *Server) readinessHandler(r *http.Request) (any, error) {
	if strings.TrimSpace(r.Method) != http.MethodGet {
		return nil, NewError(http.StatusMethodNotAllowed, errBadRequest, "method not allowed")
	}
	if s.store == nil {
		return nil, NewError(http.StatusServiceUnavailable, errInternal, "api dependencies are not configured")
	}

	response := map[string]any{
		"status": "ok",
		"checks": map[string]any{
			"database": "ok",
			"api_mode": "ready",
		},
	}

	return response, nil
}

func (s *Server) versionHandler(r *http.Request) (any, error) {
	if strings.TrimSpace(r.Method) != http.MethodGet && strings.TrimSpace(r.Method) != http.MethodPost {
		return nil, NewError(http.StatusMethodNotAllowed, errBadRequest, "method not allowed")
	}
	return map[string]any{
		"api":     "v1",
		"version": s.version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) providerAccountsListHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if err != nil {
		return nil, err
	}
	environmentValue := strings.TrimSpace(r.URL.Query().Get("environment"))
	if environmentValue != "" {
		env := domain.Environment(environmentValue)
		if env != domain.EnvironmentSandbox && env != domain.EnvironmentProduction {
			return nil, NewError(http.StatusBadRequest, errBadRequest, "environment must be sandbox or production")
		}
	}
	environment := domain.Environment(environmentValue)

	accounts, err := store.ListProviderAccounts(r.Context())
	if err != nil {
		return nil, err
	}

	filtered := make([]map[string]any, 0, len(accounts))
	for _, account := range accounts {
		if provider != "" && account.ProviderCode != provider {
			continue
		}
		if environment != "" && account.Environment != environment {
			continue
		}
		filtered = append(filtered, renderProviderAccount(account))
	}

	return filtered, nil
}

func (s *Server) providerAccountsCreateHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	var payload providerAccountCreatePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	provider, err := parseProvider(payload.Provider)
	if err != nil {
		return nil, err
	}
	environment, err := parseEnvironment(payload.Environment)
	if err != nil {
		return nil, err
	}
	if len(payload.CredentialJSON) == 0 {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "credential_json is required")
	}

	accountID, err := store.UpsertProviderAccount(r.Context(), domain.ProviderAccount{
		ProviderCode:   provider,
		Environment:    environment,
		DisplayName:    strings.TrimSpace(payload.DisplayName),
		CredentialJSON: canonicalizeJSON(payload.CredentialJSON),
		ConfigJSON:     canonicalizeJSON(payload.ConfigJSON),
	})
	if err != nil {
		return nil, err
	}

	account, err := store.GetProviderAccountByID(r.Context(), accountID)
	if err != nil {
		return nil, err
	}
	return renderProviderAccount(account), nil
}

func (s *Server) providerAccountUpdateHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "provider account id is required")
	}

	existing, err := store.GetProviderAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "provider account not found")
		}
		return nil, err
	}

	var payload providerAccountUpdatePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	if payload.Provider != "" {
		provider, err := parseProvider(payload.Provider)
		if err != nil {
			return nil, err
		}
		existing.ProviderCode = provider
	}
	if payload.Environment != "" {
		environment, err := parseEnvironment(payload.Environment)
		if err != nil {
			return nil, err
		}
		existing.Environment = environment
	}
	if strings.TrimSpace(payload.DisplayName) != "" {
		existing.DisplayName = strings.TrimSpace(payload.DisplayName)
	}
	if payload.CredentialJSON != nil {
		existing.CredentialJSON = canonicalizeJSON(payload.CredentialJSON)
	}
	if payload.ConfigJSON != nil {
		existing.ConfigJSON = canonicalizeJSON(payload.ConfigJSON)
	}

	if strings.TrimSpace(existing.DisplayName) == "" {
		existing.DisplayName = string(existing.ProviderCode) + " " + string(existing.Environment)
	}

	if err := store.UpdateProviderAccountByID(r.Context(), existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "provider account not found")
		}
		if strings.Contains(strings.ToLower(err.Error()), "provider_id") {
			return nil, NewError(http.StatusBadRequest, errBadRequest, "invalid provider code")
		}
		if strings.Contains(strings.ToLower(err.Error()), "conflict") {
			return nil, NewError(http.StatusConflict, errConflict, "provider account already exists for this provider and environment")
		}
		return nil, err
	}

	updated, err := store.GetProviderAccountByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "provider account not found")
		}
		return nil, err
	}
	return renderProviderAccount(updated), nil
}

func (s *Server) providerAccountDeleteHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "provider account id is required")
	}
	if err := store.DeleteProviderAccount(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "provider account not found")
		}
		return nil, err
	}

	return map[string]any{"deleted": true, "id": id}, nil
}

func (s *Server) paymentsListHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if err != nil {
		return nil, err
	}
	environmentValue := strings.TrimSpace(r.URL.Query().Get("environment"))
	if environmentValue != "" && environmentValue != string(domain.EnvironmentSandbox) && environmentValue != string(domain.EnvironmentProduction) {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "environment must be sandbox or production")
	}
	environment := domain.Environment(environmentValue)
	status := domain.PaymentStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	limit := parseLimit(r.URL.Query().Get("limit"))
	offset := parseOffset(r.URL.Query().Get("offset"))

	paymentIntents, err := store.ListPaymentIntents(r.Context(), provider, environment, status, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := store.CountPaymentIntents(r.Context(), provider, environment, status)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0, len(paymentIntents))
	for _, intent := range paymentIntents {
		result = append(result, renderPaymentIntent(intent))
	}

	return map[string]any{
		"items":  result,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}, nil
}

func (s *Server) paymentsCreateHandler(r *http.Request) (any, error) {
	service, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}

	var payload paymentCreatePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	provider, err := parseProvider(payload.Provider)
	if err != nil {
		return nil, err
	}
	environment, err := parseEnvironment(payload.Environment)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Reference) == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "reference is required")
	}
	if payload.Amount <= 0 {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "amount must be greater than zero")
	}

	baseURL := strings.TrimSpace(payload.BaseURL)
	currency := strings.TrimSpace(payload.Currency)
	if currency == "" {
		currency = "IDR"
	}
	request := paymentsvc.CreateInput{
		Provider:        provider,
		Environment:     environment,
		BaseURL:         baseURL,
		ExternalRef:     strings.TrimSpace(payload.Reference),
		Amount:          payload.Amount,
		Currency:        currency,
		Method:          strings.TrimSpace(payload.Method),
		Channel:         strings.TrimSpace(payload.Channel),
		CustomerName:    strings.TrimSpace(payload.CustomerName),
		CustomerEmail:   strings.TrimSpace(payload.CustomerEmail),
		CustomerPhone:   strings.TrimSpace(payload.CustomerPhone),
		CardToken:       strings.TrimSpace(payload.CardToken),
		NotificationURL: strings.TrimSpace(payload.NotificationURL),
	}

	cacheKey := idempotencyCacheKey("payments/create", r.Header.Get("Idempotency-Key"), strings.TrimSpace(payload.Reference), string(provider), string(environment))
	if cached, ok := s.readIdempotent(cacheKey); ok {
		return cached, nil
	}

	result, err := service.Create(r.Context(), request)
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}
	response := map[string]any{
		"provider":           string(result.ProviderCode),
		"reference":          result.Reference,
		"status":             string(result.Response.Status),
		"provider_reference": result.Response.ProviderReference,
		"payment_session_id": result.Response.PaymentSessionID,
		"payment_request_id": result.Response.PaymentRequestID,
		"order_id":           result.Response.OrderID,
		"transaction_id":     result.Response.TransactionID,
		"redirect_url":       result.Response.RedirectURL,
		"raw_request":        parseRawJSON(result.Response.RawRequestJSON),
		"raw_response":       parseRawJSON(result.Response.RawResponseJSON),
	}

	s.writeIdempotent(cacheKey, response)
	return response, nil
}

func (s *Server) paymentGetHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(r.PathValue("reference"))
	if reference == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "reference is required")
	}
	intent, err := store.GetPaymentIntentByExternalRef(r.Context(), reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "payment not found")
		}
		return nil, err
	}

	attempt, err := store.GetLatestPaymentAttemptByIntent(r.Context(), intent.ID, intent.ProviderCode)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	response := renderPaymentIntent(intent)
	response["latest_attempt"] = nil
	if err == nil {
		response["latest_attempt"] = map[string]any{
			"id":                 attempt.ID,
			"provider":           string(attempt.ProviderCode),
			"provider_reference": attempt.ProviderReference,
			"status":             string(attempt.Status),
			"request":            parseRawJSON(attempt.RequestJSON),
			"response":           parseRawJSON(attempt.ResponseJSON),
			"created_at":         formatTime(attempt.CreatedAt),
			"updated_at":         formatTime(attempt.UpdatedAt),
		}
	}

	response["environment"] = string(s.environment)
	return response, nil
}

func (s *Server) paymentStatusHandler(r *http.Request) (any, error) {
	service, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(r.PathValue("reference"))
	if reference == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "reference is required")
	}

	intent, err := store.GetPaymentIntentByExternalRef(r.Context(), reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "payment not found")
		}
		return nil, err
	}

	result, err := service.Status(r.Context(), paymentsvc.StatusInput{
		Provider:          intent.ProviderCode,
		Environment:       s.environment,
		Reference:         intent.ExternalRef,
		BaseURL:           strings.TrimSpace(r.URL.Query().Get("base_url")),
		ProviderReference: strings.TrimSpace(r.URL.Query().Get("provider_reference")),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "provider") && strings.Contains(strings.ToLower(err.Error()), "not configured") {
			return nil, NewError(http.StatusNotFound, errNotFound, err.Error())
		}
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	return map[string]any{
		"provider":           string(result.ProviderCode),
		"reference":          result.Reference,
		"provider_reference": result.ProviderReference,
		"status":             string(result.Response.Status),
		"raw_request":        parseRawJSON(result.Response.RawRequestJSON),
		"raw_response":       parseRawJSON(result.Response.RawResponseJSON),
	}, nil
}

func (s *Server) paymentRefundHandler(r *http.Request) (any, error) {
	service, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(r.PathValue("reference"))
	if reference == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "reference is required")
	}

	var payload paymentRefundPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	svc := service
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	intent, err := store.GetPaymentIntentByExternalRef(r.Context(), reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "payment not found")
		}
		return nil, err
	}
	if payload.ProviderReference == "" {
		payload.ProviderReference = ""
	}

	cacheKey := idempotencyCacheKey("payments/refund", r.Header.Get("Idempotency-Key"), reference, string(intent.ProviderCode), payload.RefundReference, strconv.FormatInt(payload.Amount, 10))
	if cached, ok := s.readIdempotent(cacheKey); ok {
		return cached, nil
	}

	result, err := svc.Refund(r.Context(), paymentsvc.RefundInput{
		Provider:          intent.ProviderCode,
		Environment:       s.environment,
		Reference:         reference,
		ProviderReference: strings.TrimSpace(payload.ProviderReference),
		RefundReference:   strings.TrimSpace(payload.RefundReference),
		Amount:            payload.Amount,
		Currency:          strings.TrimSpace(payload.Currency),
		Reason:            strings.TrimSpace(payload.Reason),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not configured") {
			return nil, NewError(http.StatusNotFound, errNotFound, err.Error())
		}
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	response := map[string]any{
		"provider":           string(result.ProviderCode),
		"reference":          result.Reference,
		"provider_reference": result.ProviderReference,
		"refund_reference":   result.RefundReference,
		"status":             string(result.Response.Status),
		"raw_request":        parseRawJSON(result.Response.RawRequestJSON),
		"raw_response":       parseRawJSON(result.Response.RawResponseJSON),
	}

	s.writeIdempotent(cacheKey, response)
	return response, nil
}

func (s *Server) webhookEventsListHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	signatureValid, err := parseOptionalBool(strings.TrimSpace(r.URL.Query().Get("signature_valid")))
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "invalid signature_valid value")
	}

	limit := parseListLimit(r.URL.Query().Get("limit"), 20)
	offset := parseOffset(r.URL.Query().Get("offset"))

	events, err := store.ListWebhookEvents(r.Context(), provider, status, signatureValid, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := store.CountWebhookEvents(r.Context(), provider, status, signatureValid)
	if err != nil {
		return nil, err
	}

	rendered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		rendered = append(rendered, renderWebhookEvent(event))
	}

	return map[string]any{
		"items":  rendered,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}, nil
}

func (s *Server) webhookEventGetHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "webhook event id is required")
	}

	event, err := store.GetWebhookEventByID(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "webhook event not found")
		}
		return nil, err
	}

	return renderWebhookEvent(event), nil
}

func (s *Server) webhookEventForwardingAttemptsHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "webhook event id is required")
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseListLimit(r.URL.Query().Get("limit"), 20)

	attempts, err := store.ListForwardingAttempts(r.Context(), forwarding.AttemptFilter{
		WebhookEventID: eventID,
		Status:         status,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, renderForwardingAttempt(attempt))
	}
	return items, nil
}

func (s *Server) webhookEventReplayHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "webhook event id is required")
	}

	event, err := store.GetWebhookEventByID(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "webhook event not found")
		}
		return nil, err
	}

	forwarder := forwarding.NewService(store)
	forwarderInbound := forwarding.InboundWebhook{
		WebhookEventID: eventID,
		Provider:       event.ProviderCode,
		Body:           event.PayloadJSON,
	}

	headers, err := parseHeadersJSON(event.HeadersJSON)
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}
	forwarderInbound.Headers = headers

	if err := forwarder.Forward(r.Context(), forwarderInbound); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	return map[string]any{
		"replayed": true,
		"id":       eventID,
		"provider": string(event.ProviderCode),
	}, nil
}

func (s *Server) webhookForwardingTargetsListHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if err != nil {
		return nil, err
	}
	enabledOnly, err := parseOptionalBool(strings.TrimSpace(r.URL.Query().Get("enabled_only")))
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "invalid enabled_only value")
	}

	includeDisabled := false
	if enabledOnly != nil {
		includeDisabled = !*enabledOnly
	}

	service := forwardingsvc.New(store)
	targets, err := service.List(r.Context(), forwardingsvc.ListInput{
		Provider:        provider,
		IncludeDisabled: includeDisabled,
	})
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		items = append(items, renderForwardingTarget(target))
	}
	return items, nil
}

func (s *Server) webhookForwardingTargetCreateHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	var payload webhookForwardingTargetPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	provider, err := parseProvider(payload.Provider)
	if err != nil {
		return nil, err
	}
	if provider == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "provider is required")
	}

	hdrs, err := parseHeadersPayload(payload.Headers)
	if err != nil {
		return nil, err
	}
	eventFilter, err := parseEventFilter(payload.EventFilter)
	if err != nil {
		return nil, err
	}
	retryPolicy, err := parseRetryPolicy(payload.RetryPolicy)
	if err != nil {
		return nil, err
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	service := forwardingsvc.New(store)
	targetID, err := service.Add(r.Context(), forwardingsvc.AddInput{
		Provider:    provider,
		Name:        strings.TrimSpace(payload.Name),
		URL:         strings.TrimSpace(payload.URL),
		Enabled:     enabled,
		Headers:     hdrs,
		EventFilter: eventFilter,
		RetryPolicy: retryPolicy,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "name") {
			return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
		}
		return nil, err
	}

	target, err := store.GetForwardingTarget(r.Context(), targetID)
	if err != nil {
		return nil, err
	}
	return renderForwardingTarget(target), nil
}

func (s *Server) webhookForwardingTargetUpdateHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "forwarding target id is required")
	}

	var payload webhookForwardingTargetUpdatePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	input := forwardingsvc.UpdateInput{ID: id}
	if payload.Name != nil {
		input.Name = strings.TrimSpace(*payload.Name)
	}
	if payload.URL != nil {
		input.URL = strings.TrimSpace(*payload.URL)
	}
	if payload.Enabled != nil {
		input.Enabled = *payload.Enabled
		input.EnabledSet = true
	}
	if payload.Headers != nil {
		headers, err := parseHeadersPayload(*payload.Headers)
		if err != nil {
			return nil, err
		}
		input.Headers = headers
		input.HeadersSet = true
	}
	if payload.EventFilter != nil {
		eventFilter, err := parseEventFilter(*payload.EventFilter)
		if err != nil {
			return nil, err
		}
		input.EventFilter = eventFilter
		input.EventFilterSet = true
	}
	if payload.RetryPolicy != nil {
		retryPolicy, err := parseRetryPolicy(*payload.RetryPolicy)
		if err != nil {
			return nil, err
		}
		input.RetryPolicy = retryPolicy
		input.RetryPolicySet = true
	}

	service := forwardingsvc.New(store)
	if err := service.Update(r.Context(), input); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "forwarding target not found")
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, NewError(http.StatusNotFound, errNotFound, "forwarding target not found")
		}
		if strings.Contains(strings.ToLower(err.Error()), "name") || strings.Contains(strings.ToLower(err.Error()), "target url") {
			return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
		}
		return nil, err
	}

	target, err := store.GetForwardingTarget(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "forwarding target not found")
		}
		return nil, err
	}

	return renderForwardingTarget(target), nil
}

func (s *Server) webhookForwardingTargetDeleteHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "forwarding target id is required")
	}

	service := forwardingsvc.New(store)
	if err := service.Remove(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewError(http.StatusNotFound, errNotFound, "forwarding target not found")
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, NewError(http.StatusNotFound, errNotFound, "forwarding target not found")
		}
		return nil, err
	}

	return map[string]any{
		"deleted": true,
		"id":      id,
	}, nil
}

func (s *Server) webhookForwardingAttemptsListHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if err != nil {
		return nil, err
	}
	filter := forwarding.AttemptFilter{
		Provider:       provider,
		TargetID:       strings.TrimSpace(r.URL.Query().Get("target_id")),
		WebhookEventID: strings.TrimSpace(r.URL.Query().Get("event_id")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:          parseListLimit(r.URL.Query().Get("limit"), 20),
	}

	attempts, err := store.ListForwardingAttempts(r.Context(), filter)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, renderForwardingAttempt(attempt))
	}
	return items, nil
}

func (s *Server) paymentReconcileHandler(r *http.Request) (any, error) {
	service, err := s.requirePaymentService()
	if err != nil {
		return nil, err
	}

	provider, err := parseProvider(strings.TrimSpace(r.PathValue("provider")))
	if err != nil {
		return nil, err
	}
	if provider == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "provider is required")
	}

	reference := strings.TrimSpace(r.PathValue("reference"))
	if reference == "" {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "reference is required")
	}

	result, err := service.Reconcile(r.Context(), paymentsvc.ReconcileInput{
		Provider:          provider,
		Environment:       s.environment,
		BaseURL:           strings.TrimSpace(r.URL.Query().Get("base_url")),
		Reference:         reference,
		ProviderReference: strings.TrimSpace(r.URL.Query().Get("provider_reference")),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not configured") {
			return nil, NewError(http.StatusNotFound, errNotFound, err.Error())
		}
		return nil, NewError(http.StatusBadRequest, errBadRequest, err.Error())
	}

	response := map[string]any{
		"provider":           string(result.ProviderCode),
		"reference":          result.Reference,
		"provider_reference": result.ProviderReference,
		"local_status":       string(result.LocalStatus),
		"provider_status":    string(result.ProviderStatus),
		"matched":            result.Matched,
		"updated":            result.Updated,
	}
	if result.Matched {
		response["message"] = "status matches"
	} else if result.Updated {
		response["message"] = "status updated"
	} else {
		response["message"] = "status mismatch"
	}
	response["raw_request"] = parseRawJSON(result.Response.RawRequestJSON)
	response["raw_response"] = parseRawJSON(result.Response.RawResponseJSON)

	return response, nil
}

func (s *Server) statsHandler(r *http.Request) (any, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}

	paymentIntents, err := store.CountPaymentIntents(r.Context(), "", "", "")
	if err != nil {
		return nil, err
	}
	webhookEvents, err := store.CountWebhookEvents(r.Context(), "", "", nil)
	if err != nil {
		return nil, err
	}
	forwardingTargets, err := store.CountForwardingTargets(r.Context(), "", false)
	if err != nil {
		return nil, err
	}
	forwardingAttempts, err := store.CountForwardingAttempts(r.Context(), forwarding.AttemptFilter{Limit: 1})
	if err != nil {
		return nil, err
	}

	providerAccounts, err := store.ListProviderAccounts(r.Context())
	if err != nil {
		return nil, err
	}

	totals := map[string]any{
		"provider_accounts":           len(providerAccounts),
		"payment_intents":             paymentIntents,
		"webhook_events":              webhookEvents,
		"webhook_forwarding_targets":  forwardingTargets,
		"webhook_forwarding_attempts": forwardingAttempts,
	}

	return map[string]any{
		"totals":        totals,
		"environment":   string(s.environment),
		"database_path": s.databasePath,
		"updated_at":    time.Now().UTC().Format(time.RFC3339),
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

func newRequestID() string {
	seq := atomic.AddUint64(&requestIDSeq, 1)
	return fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), seq)
}

type idempotencyEntry struct {
	payload any
	expires time.Time
}

func parseProvider(value string) (domain.ProviderCode, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	candidate := domain.ProviderCode(strings.ToLower(trimmed))
	supported := domain.SupportedProviders()
	for _, item := range supported {
		if candidate == item {
			return candidate, nil
		}
	}

	return "", NewError(http.StatusBadRequest, errBadRequest, "provider must be one of \"midtrans\", \"xendit\", \"doku\", \"ipaymu\"")
}

func parseEnvironment(raw string) (domain.Environment, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return domain.EnvironmentSandbox, nil
	}
	environment := domain.Environment(raw)
	if environment != domain.EnvironmentSandbox && environment != domain.EnvironmentProduction {
		return "", NewError(http.StatusBadRequest, errBadRequest, "environment must be sandbox or production")
	}
	return environment, nil
}

func decodeJSONBody(r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return NewError(http.StatusBadRequest, errBadRequest, "invalid JSON body")
	}
	return nil
}

func parseLimit(raw string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return 100
	}
	if parsed > 200 {
		return 200
	}
	return parsed
}

func parseOffset(raw string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func parseListLimit(raw string, defaultValue int) int {
	if defaultValue <= 0 {
		defaultValue = 20
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	if parsed > 200 {
		return 200
	}
	return parsed
}

func parseOptionalBool(raw string) (*bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	value, err := strconv.ParseBool(trimmed)
	if err != nil {
		return nil, NewError(http.StatusBadRequest, errBadRequest, "invalid boolean value")
	}
	return &value, nil
}

func parseHeadersPayload(raw map[string]any) (http.Header, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	headers := make(http.Header)
	for key, rawValue := range raw {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}

		switch values := rawValue.(type) {
		case nil:
			continue
		case string:
			headers.Add(normalizedKey, strings.TrimSpace(values))
		case []string:
			for _, value := range values {
				headers.Add(normalizedKey, strings.TrimSpace(value))
			}
		case []any:
			for _, value := range values {
				encoded, err := parseSimpleString(value)
				if err != nil {
					return nil, fmt.Errorf("invalid header value for %s", normalizedKey)
				}
				headers.Add(normalizedKey, encoded)
			}
		case map[string]any, map[string]string:
			return nil, fmt.Errorf("invalid header value for %s", normalizedKey)
		default:
			encoded, err := parseSimpleString(values)
			if err != nil {
				return nil, fmt.Errorf("invalid header value for %s", normalizedKey)
			}
			headers.Add(normalizedKey, encoded)
		}
	}

	return headers, nil
}

func parseSimpleString(raw any) (string, error) {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value), nil
	case fmt.Stringer:
		return strings.TrimSpace(value.String()), nil
	case nil:
		return "", nil
	default:
		normalized := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if normalized == "" {
			return "", fmt.Errorf("empty")
		}
		return normalized, nil
	}
}

func parseEventFilter(raw map[string]any) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}

	filter := make(map[string]string, len(raw))
	for key, rawValue := range raw {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}

		parsed, err := parseSimpleString(rawValue)
		if err != nil {
			return nil, fmt.Errorf("event_filter[%s] must be a string", normalizedKey)
		}
		filter[normalizedKey] = parsed
	}

	return filter, nil
}

func parseRetryPolicy(payload webhookForwardingRetryPolicyPayload) (forwarding.RetryPolicy, error) {
	if payload.MaxAttempts < 0 {
		return forwarding.RetryPolicy{}, fmt.Errorf("retry policy max_attempts cannot be negative")
	}
	if payload.TimeoutMilliseconds < 0 || payload.TimeoutSeconds < 0 {
		return forwarding.RetryPolicy{}, fmt.Errorf("retry policy timeout cannot be negative")
	}
	if payload.BackoffMilliseconds < 0 || payload.BackoffSeconds < 0 {
		return forwarding.RetryPolicy{}, fmt.Errorf("retry policy backoff cannot be negative")
	}

	timeout := parseRetryDuration(payload.TimeoutMilliseconds, payload.TimeoutSeconds)
	backoff := parseRetryDuration(payload.BackoffMilliseconds, payload.BackoffSeconds)

	return forwarding.RetryPolicy{
		MaxAttempts: payload.MaxAttempts,
		Timeout:     timeout,
		Backoff:     backoff,
	}, nil
}

func parseRetryDuration(milliseconds, seconds int64) time.Duration {
	if milliseconds > 0 {
		return time.Duration(milliseconds) * time.Millisecond
	}
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

func renderHeaders(header http.Header) map[string]any {
	if len(header) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(header))
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			out[key] = values[0]
			continue
		}
		out[key] = values
	}
	return out
}

func renderRetryPolicy(policy forwarding.RetryPolicy) map[string]any {
	return map[string]any{
		"max_attempts": policy.MaxAttempts,
		"timeout_ms":   int(policy.Timeout.Milliseconds()),
		"backoff_ms":   int(policy.Backoff.Milliseconds()),
	}
}

func renderWebhookEvent(event domain.WebhookEvent) map[string]any {
	processedAt := ""
	if event.ProcessedAt != nil {
		processedAt = formatTime(*event.ProcessedAt)
	}

	return map[string]any{
		"id":                event.ID,
		"provider":          string(event.ProviderCode),
		"provider_event_id": event.ProviderEventID,
		"event_type":        event.EventType,
		"signature_valid":   event.SignatureValid,
		"processing_status": event.ProcessingStatus,
		"received_at":       formatTime(event.ReceivedAt),
		"processed_at":      processedAt,
		"payload_json":      parseRawJSON(event.PayloadJSON),
		"headers_json":      parseRawJSON(event.HeadersJSON),
	}
}

func renderForwardingAttempt(attempt forwarding.AttemptRecord) map[string]any {
	return map[string]any{
		"id":               attempt.ID,
		"webhook_event_id": attempt.WebhookEventID,
		"target_id":        attempt.TargetID,
		"target_name":      attempt.TargetName,
		"target_url":       attempt.TargetURL,
		"provider":         string(attempt.Provider),
		"status":           attempt.Status,
		"attempt_no":       attempt.AttemptNo,
		"request_json":     parseRawJSON(attempt.RequestJSON),
		"response_json":    parseRawJSON(attempt.ResponseJSON),
		"created_at":       formatTime(attempt.CreatedAt),
		"updated_at":       formatTime(attempt.UpdatedAt),
	}
}

func renderForwardingTarget(target forwarding.Target) map[string]any {
	return map[string]any{
		"id":           target.ID,
		"provider":     string(target.Provider),
		"name":         target.Name,
		"url":          target.URL,
		"headers":      renderHeaders(target.Headers),
		"event_filter": target.EventFilter,
		"retry_policy": renderRetryPolicy(target.RetryPolicy),
		"enabled":      target.Enabled,
	}
}

func parseHeadersJSON(raw json.RawMessage) (http.Header, error) {
	if len(raw) == 0 {
		return http.Header{}, nil
	}

	rawEntries := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return nil, err
	}

	headers := make(http.Header, len(rawEntries))
	for key, rawValue := range rawEntries {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}

		var multiValues []string
		if err := json.Unmarshal(rawValue, &multiValues); err == nil {
			for _, value := range multiValues {
				headers.Add(trimmedKey, value)
			}
			continue
		}

		var singleValue string
		if err := json.Unmarshal(rawValue, &singleValue); err == nil {
			headers.Set(trimmedKey, singleValue)
			continue
		}

		return nil, fmt.Errorf("invalid stored header value for %s", trimmedKey)
	}

	return headers, nil
}

func renderProviderAccount(account domain.ProviderAccount) map[string]any {
	return map[string]any{
		"id":              account.ID,
		"provider":        string(account.ProviderCode),
		"environment":     string(account.Environment),
		"display_name":    account.DisplayName,
		"credential_json": parseRawJSON(account.CredentialJSON),
		"config_json":     parseRawJSON(account.ConfigJSON),
		"created_at":      formatTime(account.CreatedAt),
		"updated_at":      formatTime(account.UpdatedAt),
	}
}

func renderPaymentIntent(intent domain.PaymentIntent) map[string]any {
	return map[string]any{
		"id":            intent.ID,
		"reference":     intent.ExternalRef,
		"provider":      string(intent.ProviderCode),
		"amount":        intent.Amount,
		"currency":      intent.Currency,
		"status":        string(intent.Status),
		"metadata_json": parseRawJSON(intent.MetadataJSON),
		"created_at":    formatTime(intent.CreatedAt),
		"updated_at":    formatTime(intent.UpdatedAt),
	}
}

func parseRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return string(raw)
	}
	return payload
}

func canonicalizeJSON(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return []byte("{}")
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return []byte("{}")
	}
	return json.RawMessage(trimmed)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func idempotencyCacheKey(operation, key string, refs ...string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	if len(refs) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(refs))
	for _, ref := range refs {
		normalized = append(normalized, strings.TrimSpace(ref))
	}
	return operation + ":" + strings.TrimSpace(key) + ":" + strings.Join(normalized, ":")
}

func (s *Server) readIdempotent(cacheKey string) (any, bool) {
	if strings.TrimSpace(cacheKey) == "" {
		return nil, false
	}
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	entry, ok := s.idempotencyMap[cacheKey]
	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(entry.expires) {
		delete(s.idempotencyMap, cacheKey)
		return nil, false
	}
	return entry.payload, true
}

func (s *Server) writeIdempotent(cacheKey string, payload any) {
	if strings.TrimSpace(cacheKey) == "" {
		return
	}
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	now := time.Now().UTC()
	if len(s.idempotencyMap) >= idemLimit {
		s.pruneIdempotency(now)
	}
	if len(s.idempotencyMap) >= idemLimit {
		for key := range s.idempotencyMap {
			delete(s.idempotencyMap, key)
			if len(s.idempotencyMap) < idemLimit/2 {
				break
			}
		}
	}
	s.idempotencyMap[cacheKey] = idempotencyEntry{
		payload: payload,
		expires: now.Add(idemTTL),
	}
}

func (s *Server) pruneIdempotency(now time.Time) {
	for cacheKey, entry := range s.idempotencyMap {
		if now.After(entry.expires) {
			delete(s.idempotencyMap, cacheKey)
		}
	}
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewError(status int, code, message string) error {
	return &apiError{Status: status, Code: code, Message: message}
}

type providerAccountCreatePayload struct {
	Provider       string          `json:"provider"`
	Environment    string          `json:"environment"`
	DisplayName    string          `json:"display_name"`
	CredentialJSON json.RawMessage `json:"credential_json"`
	ConfigJSON     json.RawMessage `json:"config_json"`
}

type providerAccountUpdatePayload struct {
	Provider       string          `json:"provider"`
	Environment    string          `json:"environment"`
	DisplayName    string          `json:"display_name"`
	CredentialJSON json.RawMessage `json:"credential_json"`
	ConfigJSON     json.RawMessage `json:"config_json"`
}

type paymentCreatePayload struct {
	Provider        string `json:"provider"`
	Environment     string `json:"environment"`
	Reference       string `json:"reference"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Method          string `json:"method"`
	Channel         string `json:"channel"`
	CustomerName    string `json:"customer_name"`
	CustomerEmail   string `json:"customer_email"`
	CustomerPhone   string `json:"customer_phone"`
	CardToken       string `json:"card_token"`
	NotificationURL string `json:"notification_url"`
	BaseURL         string `json:"base_url"`
}

type paymentRefundPayload struct {
	ProviderReference string `json:"provider_reference"`
	RefundReference   string `json:"refund_reference"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	Reason            string `json:"reason"`
}

type webhookForwardingRetryPolicyPayload struct {
	MaxAttempts         int   `json:"max_attempts"`
	TimeoutMilliseconds int64 `json:"timeout_ms"`
	TimeoutSeconds      int64 `json:"timeout_sec"`
	BackoffMilliseconds int64 `json:"backoff_ms"`
	BackoffSeconds      int64 `json:"backoff_sec"`
}

type webhookForwardingTargetPayload struct {
	Provider    string                              `json:"provider"`
	Name        string                              `json:"name"`
	URL         string                              `json:"url"`
	Headers     map[string]any                      `json:"headers"`
	EventFilter map[string]any                      `json:"event_filter"`
	RetryPolicy webhookForwardingRetryPolicyPayload `json:"retry_policy"`
	Enabled     *bool                               `json:"enabled"`
}

type webhookForwardingTargetUpdatePayload struct {
	Name        *string                              `json:"name"`
	URL         *string                              `json:"url"`
	Headers     *map[string]any                      `json:"headers"`
	EventFilter *map[string]any                      `json:"event_filter"`
	RetryPolicy *webhookForwardingRetryPolicyPayload `json:"retry_policy"`
	Enabled     *bool                                `json:"enabled"`
}

// AuditStore defines the optional destination for request audit logs.
type AuditStore interface {
	RecordAuditEvent(context.Context, auditlog.Event) error
}
