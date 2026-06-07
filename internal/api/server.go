package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

const (
	errBadRequest = "bad_request"
	requestIDHeader = "X-Request-ID"
	requestIDQuery = "request_id"
)

// Server handles HTTP API routes used by daemon API mode.
type Server struct {
	version     string
	store       Store
	payments    PaymentService
	environment domain.Environment
}

type Config struct {
	Version            string
	Store              Store
	PaymentService     PaymentService
	DefaultEnvironment domain.Environment
}

type Store interface{}
type PaymentService interface{}

func NewServer(cfg Config) *Server {
	return &Server{
		version:     strings.TrimSpace(cfg.Version),
		store:       cfg.Store,
		payments:    cfg.PaymentService,
		environment: defaultEnvironment(cfg.DefaultEnvironment),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.wrap(s.health, false))
	mux.HandleFunc("GET /api/v1/healthz", s.wrap(s.health, false))
	mux.HandleFunc("GET /api/v1/version", s.wrap(s.versionHandler, true))
	mux.HandleFunc("POST /api/v1/version", s.wrap(s.versionHandler, true))
	mux.Handle("/", http.NotFoundHandler())
	return mux
}

type endpointFunc func(*http.Request) (any, error)

func (s *Server) wrap(fn endpointFunc, requireAuth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := r.Header.Get(requestIDHeader)
		if strings.TrimSpace(requestID) == "" {
			requestID = newRequestID()
		}
		_ = start
		w.Header().Set(requestIDHeader, requestID)

		if requireAuth {
			if strings.TrimSpace(r.Header.Get("X-API-Key")) != "" {
				// placeholder gate for API-only auth added in Issue #75
			}
		}

		payload, err := fn(r)
		if err != nil {
			if apiErr, ok := err.(*apiError); ok {
				s.writeJSONError(w, requestID, apiErr.Status, apiErr.Code, apiErr.Message)
				return
			}
			s.writeJSONError(w, requestID, http.StatusBadRequest, errBadRequest, err.Error())
			return
		}
		s.writeJSON(w, requestID, http.StatusOK, payload)
	}
}

func (s *Server) health(_ *http.Request) (any, error) {
	return map[string]string{"status": "ok"}, nil
}

func (s *Server) versionHandler(_ *http.Request) (any, error) {
	return map[string]any{
		"api":     "v1",
		"version": s.version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) writeJSONError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"error_code": code,
		"message":    message,
		"details": map[string]any{
			"path": "/",
		},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, requestID string, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
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

func (s *Server) requireStore() (Store, error) {
	if s == nil || s.store == nil {
		return nil, NewError(http.StatusServiceUnavailable, "internal_error", "api store is not configured")
	}
	return s.store, nil
}

func (s *Server) requirePaymentService() (PaymentService, error) {
	if s == nil || s.payments == nil {
		return nil, NewError(http.StatusServiceUnavailable, "internal_error", "payment service is not configured")
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
	return time.Now().UTC().Format(time.RFC3339Nano)
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
