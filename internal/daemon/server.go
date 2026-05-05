package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pena-digital/rute-bayar/internal/domain"
	"github.com/pena-digital/rute-bayar/internal/forwarding"
)

type Server struct {
	addr      string
	forwarder *forwarding.Service
}

func NewServer(addr string, forwarder *forwarding.Service) *Server {
	if addr == "" {
		addr = ":8080"
	}
	return &Server{addr: addr, forwarder: forwarder}
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

	inbound := forwarding.InboundWebhook{
		Provider: providerCode,
		Headers:  r.Header.Clone(),
		Body:     body,
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
