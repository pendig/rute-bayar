package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
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

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	var payload map[string]any
	if err := json.NewDecoder(recorderRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["warning"] == nil {
		t.Fatalf("warning expected, got nil")
	}
}

func TestWebhookReconcilesPaymentIntentStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-001",
		ProviderCode: domain.ProviderMidtrans,
		Amount:       10000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}

	server := NewServer(":8080", store, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{
					ProviderEventID: "evt-001",
					EventType:       "capture",
					PaymentRef:      "rb-001",
					Status:          domain.PaymentStatusPaid,
				}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorder.Code, http.StatusAccepted)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusPaid {
		t.Fatalf("intent status = %q, want %q", intent.Status, domain.PaymentStatusPaid)
	}
}

func TestWebhookMarksUnmatchedWebhookReference(t *testing.T) {
	t.Parallel()

	store := &stubReconciler{intents: make(map[string]domain.PaymentIntent)}
	server := NewServer(":8080", store, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{
					ProviderEventID: "evt-002",
					EventType:       "capture",
					PaymentRef:      "missing-ref",
					Status:          domain.PaymentStatusPaid,
				}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	if len(store.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(store.events))
	}
	if store.events[0].ProcessingStatus != "unmatched" {
		t.Fatalf("ProcessingStatus = %q, want unmatched", store.events[0].ProcessingStatus)
	}
}

func TestWebhookMarksDuplicateWebhookStatus(t *testing.T) {
	t.Parallel()

	store := &stubReconciler{intents: map[string]domain.PaymentIntent{
		"rb-002": {
			ID:           "intent-002",
			ExternalRef:  "rb-002",
			ProviderCode: domain.ProviderMidtrans,
			Amount:       10000,
			Currency:     "IDR",
			Status:       domain.PaymentStatusPaid,
		},
	}}
	server := NewServer(":8080", store, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{
					ProviderEventID: "evt-dup",
					EventType:       "capture",
					PaymentRef:      "rb-002",
					Status:          domain.PaymentStatusPaid,
				}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	if len(store.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(store.events))
	}
	if store.events[0].ProcessingStatus != "duplicate" {
		t.Fatalf("ProcessingStatus = %q, want duplicate", store.events[0].ProcessingStatus)
	}

	intent := store.intents["rb-002"]
	if intent.Status != domain.PaymentStatusPaid {
		t.Fatalf("intent status = %q, want %q", intent.Status, domain.PaymentStatusPaid)
	}
}

func TestWebhookMarksReconciledStatus(t *testing.T) {
	t.Parallel()

	store := &stubReconciler{intents: map[string]domain.PaymentIntent{
		"rb-003": {
			ID:           "intent-003",
			ExternalRef:  "rb-003",
			ProviderCode: domain.ProviderMidtrans,
			Amount:       10000,
			Currency:     "IDR",
			Status:       domain.PaymentStatusPending,
		},
	}}
	server := NewServer(":8080", store, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{
					ProviderEventID: "evt-003",
					EventType:       "capture",
					PaymentRef:      "rb-003",
					Status:          domain.PaymentStatusPaid,
				}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	if len(store.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(store.events))
	}
	if store.events[0].ProcessingStatus != "reconciled" {
		t.Fatalf("ProcessingStatus = %q, want reconciled", store.events[0].ProcessingStatus)
	}

	intent := store.intents["rb-003"]
	if intent.Status != domain.PaymentStatusPaid {
		t.Fatalf("intent status = %q, want %q", intent.Status, domain.PaymentStatusPaid)
	}
}

func TestWebhookMarksUnmatchedWebhookProviderCode(t *testing.T) {
	t.Parallel()

	store := &stubReconciler{intents: map[string]domain.PaymentIntent{
		"rb-004": {
			ID:           "intent-004",
			ExternalRef:  "rb-004",
			ProviderCode: domain.ProviderXendit,
			Amount:       10000,
			Currency:     "IDR",
			Status:       domain.PaymentStatusPending,
		},
	}}
	server := NewServer(":8080", store, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{
					ProviderEventID: "evt-004",
					EventType:       "capture",
					PaymentRef:      "rb-004",
					Status:          domain.PaymentStatusPaid,
				}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/webhooks/midtrans", nil)
	recorderRecorder := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{provider}", server.webhook)
	mux.ServeHTTP(recorderRecorder, request)

	if recorderRecorder.Code != http.StatusAccepted {
		t.Fatalf("Status = %d, want %d", recorderRecorder.Code, http.StatusAccepted)
	}
	if len(store.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(store.events))
	}
	if store.events[0].ProcessingStatus != "unmatched" {
		t.Fatalf("ProcessingStatus = %q, want unmatched", store.events[0].ProcessingStatus)
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

type stubReconciler struct {
	intents map[string]domain.PaymentIntent
	events  []domain.WebhookEvent
}

func (s *stubReconciler) RecordWebhookEvent(_ context.Context, event domain.WebhookEvent) (string, error) {
	s.events = append(s.events, event)
	return "webhook-123", nil
}

func (s *stubReconciler) GetPaymentIntentByExternalRef(_ context.Context, externalRef string) (domain.PaymentIntent, error) {
	intent, ok := s.intents[externalRef]
	if !ok {
		return domain.PaymentIntent{}, sql.ErrNoRows
	}
	return intent, nil
}

func (s *stubReconciler) UpsertPaymentIntent(_ context.Context, intent domain.PaymentIntent) (string, error) {
	s.intents[strings.TrimSpace(intent.ExternalRef)] = intent
	return intent.ID, nil
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
