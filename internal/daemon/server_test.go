package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	server := NewServer(":8080", recorder, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return errors.New("webhook signature mismatch")
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusBadRequest)
	}

	var payload map[string]any
	if err := json.NewDecoder(recorderRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "rejected" {
		t.Fatalf("status = %v, want rejected", payload["status"])
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].ProcessingStatus != "rejected" {
		t.Fatalf("ProcessingStatus = %q, want rejected", recorder.events[0].ProcessingStatus)
	}
	if recorder.events[0].SignatureValid {
		t.Fatalf("SignatureValid = true, want false")
	}
}

func TestWebhookReturnsBadRequestOnParseFailure(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	server := NewServer(":8080", recorder, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{}, errors.New("parse error")
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusBadRequest)
	}
	var payload map[string]any
	if err := json.NewDecoder(recorderRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["warning"] == nil {
		t.Fatalf("warning expected, got nil")
	}
}

func TestWebhookPassesThroughWithoutHandler(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	server := NewServer(":8080", recorder, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].EventType != "unknown" {
		t.Fatalf("EventType = %q, want unknown", recorder.events[0].EventType)
	}
}

type stubWebhookRecorder struct {
	events []domain.WebhookEvent
}

func (s *stubWebhookRecorder) RecordWebhookEvent(_ context.Context, event domain.WebhookEvent) (string, error) {
	s.events = append(s.events, event)
	return "webhook-123", nil
}

type testWebhookAdapter struct {
	verifyFn func(context.Context, provider.WebhookRequest) error
	parseFn  func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error)
}

func (a *testWebhookAdapter) VerifyWebhook(ctx context.Context, req provider.WebhookRequest) error {
	if a.verifyFn == nil {
		return nil
	}
	return a.verifyFn(ctx, req)
}

func (a *testWebhookAdapter) ParseWebhook(ctx context.Context, req provider.WebhookRequest) (provider.WebhookEvent, error) {
	if a.parseFn == nil {
		return provider.WebhookEvent{}, nil
	}
	return a.parseFn(ctx, req)
}

func (a *testWebhookAdapter) CreatePayment(context.Context, provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	return provider.CreatePaymentResponse{}, nil
}

func (a *testWebhookAdapter) GetPaymentStatus(context.Context, string) (provider.PaymentStatusResponse, error) {
	return provider.PaymentStatusResponse{}, nil
}

func (a *testWebhookAdapter) RefundPayment(context.Context, provider.RefundRequest) (provider.RefundResponse, error) {
	return provider.RefundResponse{}, nil
}

func (a *testWebhookAdapter) Code() domain.ProviderCode {
	return domain.ProviderMidtrans
}

func (a *testWebhookAdapter) Capabilities() []provider.Capability {
	return nil
}
