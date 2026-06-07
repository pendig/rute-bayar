package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pendig/rute-bayar/internal/auditlog"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
)

const (
	errUnauthorized = "unauthorized"
	errForbidden    = "forbidden"
	errRateLimited  = "rate_limited"
	errBadRequest   = "bad_request"
	errInternal     = "internal_error"

	requestIDHeader = "X-Request-ID"
	requestIDQuery  = "request_id"
	headerOrigin    = "Origin"

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

	rateBuckets map[string]*rateBucket
	bucketsMu   sync.Mutex
	idempotency map[string]any
	idemMu      sync.Mutex
}

type Config struct {
	Version            string
	APIKey             string
	AllowedOrigins     string
	RateLimitPerMinute int
	AuditSink          AuditStore
	Store              Store
	PaymentService     PaymentService
	DefaultEnvironment domain.Environment
}

type Store interface {
	ListProviderAccountsByFilter(context.Context, domain.ProviderCode, domain.Environment) ([]domain.ProviderAccount, error)
	UpsertProviderAccount(context.Context, domain.ProviderAccount) (string, error)
	UpdateProviderAccountByID(context.Context, string, domain.ProviderAccount) (string, error)
	DeleteProviderAccountByID(context.Context, string) error
	ListPaymentIntents(context.Context, domain.ProviderCode, domain.PaymentStatus, int, int) ([]domain.PaymentIntent, error)
	GetPaymentIntentByExternalRef(context.Context, string) (domain.PaymentIntent, error)
	GetLatestPaymentAttemptByIntent(context.Context, string, domain.ProviderCode) (domain.PaymentAttempt, error)
	ListWebhookEvents(context.Context, domain.ProviderCode, string, *bool, int, int) ([]domain.WebhookEvent, error)
	GetWebhookEventByID(context.Context, string) (domain.WebhookEvent, error)
	ListForwardingTargets(context.Context, domain.ProviderCode) ([]forwarding.Target, error)
	AddForwardingTarget(context.Context, forwarding.Target) (string, error)
	GetForwardingTarget(context.Context, string) (forwarding.Target, error)
	UpdateForwardingTarget(context.Context, forwarding.Target) error
	DeleteForwardingTarget(context.Context, string) error
	ListForwardingAttempts(context.Context, forwarding.AttemptFilter) ([]forwarding.AttemptRecord, error)
	ListEnabledTargets(context.Context, domain.ProviderCode) ([]forwarding.Target, error)
	RecordAttempt(context.Context, forwarding.Attempt) error
	CountProviderAccounts(context.Context) (int, error)
	CountPaymentIntents(context.Context) (int, error)
	CountWebhookEvents(context.Context) (int, error)
	CountForwardingTargets(context.Context) (int, error)
	CountForwardingAttempts(context.Context) (int, error)
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
		version:       strings.TrimSpace(cfg.Version),
		apiKey:        strings.TrimSpace(cfg.APIKey),
		allowedOrigin: allowed,
		rateLimit:     rate,
		auditStore:    cfg.AuditSink,
		store:         cfg.Store,
		payments:      cfg.PaymentService,
		environment:   defaultEnvironment(cfg.DefaultEnvironment),
		rateBuckets:   map[string]*rateBucket{},
		idempotency:   map[string]any{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.wrap(s.health, false))
	mux.HandleFunc("GET /api/v1/healthz", s.wrap(s.health, false))
	mux.HandleFunc("POST /api/v1/version", s.wrap(s.versionHandler, true))
	mux.HandleFunc("GET /api/v1/version", s.wrap(s.versionHandler, true))
	mux.HandleFunc("GET /api/v1/provider-accounts", s.wrap(s.listProviderAccounts, true))
	mux.HandleFunc("POST /api/v1/provider-accounts", s.wrap(s.createProviderAccount, true))
	mux.HandleFunc("PUT /api/v1/provider-accounts/{id}", s.wrap(s.updateProviderAccount, true))
	mux.HandleFunc("DELETE /api/v1/provider-accounts/{id}", s.wrap(s.deleteProviderAccount, true))
	mux.HandleFunc("GET /api/v1/payments", s.wrap(s.listPayments, true))
	mux.HandleFunc("POST /api/v1/payments", s.wrap(s.createPayment, true))
	mux.HandleFunc("GET /api/v1/payments/{reference}", s.wrap(s.getPayment, true))
	mux.HandleFunc("GET /api/v1/payments/{reference}/status", s.wrap(s.getPaymentStatus, true))
	mux.HandleFunc("POST /api/v1/payments/{reference}/refund", s.wrap(s.refundPayment, true))
	mux.HandleFunc("GET /api/v1/webhook-events", s.wrap(s.listWebhookEvents, true))
	mux.HandleFunc("GET /api/v1/webhook-events/{id}", s.wrap(s.getWebhookEvent, true))
	mux.HandleFunc("GET /api/v1/webhook-events/{id}/forwarding-attempts", s.wrap(s.listWebhookEventAttempts, true))
	mux.HandleFunc("POST /api/v1/webhook-events/{id}/replay", s.wrap(s.replayWebhookEvent, true))
	mux.HandleFunc("GET /api/v1/webhook-forwarding-targets", s.wrap(s.listForwardingTargets, true))
	mux.HandleFunc("POST /api/v1/webhook-forwarding-targets", s.wrap(s.createForwardingTarget, true))
	mux.HandleFunc("PUT /api/v1/webhook-forwarding-targets/{id}", s.wrap(s.updateForwardingTarget, true))
	mux.HandleFunc("DELETE /api/v1/webhook-forwarding-targets/{id}", s.wrap(s.deleteForwardingTarget, true))
	mux.HandleFunc("GET /api/v1/webhook-forwarding-attempts", s.wrap(s.listForwardingAttempts, true))
	mux.HandleFunc("POST /api/v1/reconcile/{provider}/{reference}", s.wrap(s.reconcilePayment, true))
	mux.HandleFunc("POST /api/v1/reconcile/{provider}", s.wrap(s.reconcilePayment, true))
	mux.HandleFunc("POST /api/v1/reconcile", s.wrap(s.reconcilePayment, true))
	mux.HandleFunc("GET /api/v1/stats", s.wrap(s.stats, true))
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
			return
		}

		if requireAuth {
			if !s.isAuthorized(r) {
				status := http.StatusUnauthorized
				code := errUnauthorized
				message := "missing or invalid API key"
				if s.apiKey != "" {
					status = http.StatusUnauthorized
					code = errUnauthorized
				} else {
					status = http.StatusForbidden
					code = errForbidden
					message = "API key disabled"
				}
				s.writeJSONError(w, r, requestID, status, code, message)
				s.audit(r, requestID, http.StatusUnauthorized, time.Since(start).Milliseconds())
				return
			}
		}

		payload, err := fn(r)
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) {
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

func (s *Server) isAuthorized(r *http.Request) bool {
	if s.apiKey == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
	return provided == s.apiKey && provided != ""
}

type rateBucket struct {
	start time.Time
	count int
}

func (s *Server) allowRate(r *http.Request, remoteAddr string) bool {
	if s.rateLimit <= 0 {
		return true
	}

	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if key == "" {
		key = remoteAddr
	}
	if key == "" {
		key = "anonymous"
	}

	now := time.Now().UTC()
	window := now.Truncate(time.Minute)

	s.bucketsMu.Lock()
	defer s.bucketsMu.Unlock()

	bucket, ok := s.rateBuckets[key]
	if !ok {
		bucket = &rateBucket{start: window}
		s.rateBuckets[key] = bucket
	}

	if bucket.start != window {
		bucket.start = window
		bucket.count = 0
	}
	if bucket.count >= s.rateLimit {
		return false
	}
	bucket.count++
	return true
}

func (s *Server) writeCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
	w.Header().Set("Vary", headerOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "3600")
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

func (s *Server) health(r *http.Request) (any, error) {
	if strings.TrimSpace(r.Method) != http.MethodGet {
		return nil, NewError(http.StatusMethodNotAllowed, errBadRequest, "method not allowed")
	}
	return map[string]string{"status": "ok"}, nil
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

func newRequestID() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// AuditStore defines the optional destination for request audit logs.
type AuditStore interface {
	RecordAuditEvent(context.Context, auditlog.Event) error
}

type AuditEvent = auditlog.Event
