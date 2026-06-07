package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/webhooksvc"
)

type WebhookRecorder = webhooksvc.Recorder

type Server struct {
	addr       string
	processor  *webhooksvc.Service
	apiHandler http.Handler
}

func NewServer(addr string, recorder WebhookRecorder, forwarder *forwarding.Service, handlers map[domain.ProviderCode]provider.Adapter) *Server {
	if addr == "" {
		addr = ":8080"
	}
	var processor *webhooksvc.Service
	if recorder != nil || forwarder != nil || len(handlers) > 0 {
		processor = webhooksvc.New(recorder, forwarder, handlers)
	}
	return &Server{addr: addr, processor: processor}
}

func (s *Server) WithAPIHandler(handler http.Handler) *Server {
	s.apiHandler = handler
	return s
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	if s.processor != nil {
		mux.HandleFunc("POST /webhooks/{provider}", s.webhook)
	}
	if s.apiHandler != nil {
		mux.Handle("/", s.apiHandler)
	}
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read webhook body"})
		return
	}

	if s == nil || s.processor == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "webhook service is not configured"})
		return
	}

	result, err := s.processor.Process(r.Context(), webhooksvc.Input{
		Provider: domain.ProviderCode(strings.TrimSpace(r.PathValue("provider"))),
		Request: provider.WebhookRequest{
			Headers:    r.Header,
			Body:       body,
			TargetPath: r.URL.Path,
		},
	})
	if err != nil && result.StatusCode == 0 {
		result.StatusCode = http.StatusInternalServerError
		result.Body = map[string]any{"error": err.Error()}
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusInternalServerError
	}
	if result.Body == nil {
		result.Body = map[string]any{"error": "webhook processing failed"}
	}
	writeJSON(w, result.StatusCode, result.Body)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
