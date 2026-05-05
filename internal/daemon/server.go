package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
)

type Server struct {
	addr            string
	webhookRecorder WebhookRecorder
	forwarder       *forwarding.Service
}

type WebhookRecorder interface {
	RecordWebhookEvent(context.Context, domain.WebhookEvent) (string, error)
}

func NewServer(addr string, recorder WebhookRecorder, forwarder *forwarding.Service) *Server {
	if addr == "" {
		addr = ":8080"
	}
	return &Server{addr: addr, webhookRecorder: recorder, forwarder: forwarder}
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
	webhookEventID := ""
	if s.webhookRecorder != nil {
		id, err := s.webhookRecorder.RecordWebhookEvent(r.Context(), domain.WebhookEvent{
			ProviderCode:     providerCode,
			EventType:        "unknown",
			SignatureValid:   false,
			PayloadJSON:      body,
			HeadersJSON:      headersJSON,
			ReceivedAt:       time.Now().UTC(),
			ProcessingStatus: "received",
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record webhook event"})
			return
		}
		webhookEventID = id
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
