package webhooksvc

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestProcessRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	svc := New(recorder, nil, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return errors.New("webhook signature mismatch")
			},
		},
	})

	result, err := svc.Process(context.Background(), Input{
		Provider: domain.ProviderMidtrans,
		Request: provider.WebhookRequest{
			Headers: http.Header{"X-Test": []string{"one"}},
			Body:    []byte(`{"order_id":"rb-001"}`),
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusBadRequest)
	}
	if got := result.Body["status"]; got != "rejected" {
		t.Fatalf("status = %v, want rejected", got)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].ProcessingStatus != "rejected" {
		t.Fatalf("ProcessingStatus = %q, want rejected", recorder.events[0].ProcessingStatus)
	}
}

func TestProcessReconcilesAndForwards(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{intents: map[string]domain.PaymentIntent{
		"rb-001": {
			ID:           "intent-001",
			ExternalRef:  "rb-001",
			ProviderCode: domain.ProviderMidtrans,
			Amount:       10000,
			Currency:     "IDR",
			Status:       domain.PaymentStatusPending,
		},
	}}
	forwarder := &stubForwarder{}
	svc := New(recorder, forwarder, map[domain.ProviderCode]provider.Adapter{
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

	result, err := svc.Process(context.Background(), Input{
		Provider: domain.ProviderMidtrans,
		Request: provider.WebhookRequest{
			Headers: http.Header{"X-Test": []string{"one"}},
			Body:    []byte(`{"order_id":"rb-001"}`),
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusAccepted)
	}
	if got := result.Body["status"]; got != "accepted" {
		t.Fatalf("status = %v, want accepted", got)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].ProcessingStatus != "reconciled" {
		t.Fatalf("ProcessingStatus = %q, want reconciled", recorder.events[0].ProcessingStatus)
	}
	if len(forwarder.inbounds) != 1 {
		t.Fatalf("Forward calls = %d, want 1", len(forwarder.inbounds))
	}
	if forwarder.inbounds[0].Provider != domain.ProviderMidtrans {
		t.Fatalf("Forward provider = %q, want midtrans", forwarder.inbounds[0].Provider)
	}
	if forwarder.inbounds[0].WebhookEventID == "" {
		t.Fatal("WebhookEventID should be populated")
	}
	if got := string(forwarder.inbounds[0].Body); got != `{"order_id":"rb-001"}` {
		t.Fatalf("forwarded body = %s, want original payload", got)
	}
	if got := forwarder.inbounds[0].Headers.Get("X-Test"); got != "one" {
		t.Fatalf("forwarded header X-Test = %q, want one", got)
	}
	intent := recorder.intents["rb-001"]
	if intent.Status != domain.PaymentStatusPaid {
		t.Fatalf("intent status = %q, want paid", intent.Status)
	}
}

func TestProcessRejectsDuplicateWebhookEvent(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{
		webhookEvents: map[string]domain.WebhookEvent{
			webhookEventLookupKey(domain.ProviderMidtrans, "evt-001"): {
				ID:               "webhook-previous",
				ProviderCode:     domain.ProviderMidtrans,
				ProviderEventID:  "evt-001",
				EventType:        "capture",
				ProcessingStatus: "processed",
			},
		},
	}
	forwarder := &stubForwarder{}
	svc := New(recorder, forwarder, map[domain.ProviderCode]provider.Adapter{
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

	result, err := svc.Process(context.Background(), Input{
		Provider: domain.ProviderMidtrans,
		Request: provider.WebhookRequest{
			Headers: http.Header{"X-Test": []string{"one"}},
			Body:    []byte(`{"order_id":"rb-001"}`),
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusAccepted)
	}
	if got := result.Body["warning"]; got != "duplicate webhook event ignored" {
		t.Fatalf("warning = %v, want duplicate webhook event ignored", got)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].ProcessingStatus != "duplicate" {
		t.Fatalf("ProcessingStatus = %q, want duplicate", recorder.events[0].ProcessingStatus)
	}
	if recorder.events[0].ProcessedAt == nil {
		t.Fatal("ProcessedAt should be populated for duplicate event")
	}
	if len(forwarder.inbounds) != 0 {
		t.Fatalf("Forward calls = %d, want 0", len(forwarder.inbounds))
	}
}

func TestProcessReturnsAcceptedOnParseFailureWithoutForwarding(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	forwarder := &stubForwarder{}
	svc := New(recorder, forwarder, map[domain.ProviderCode]provider.Adapter{
		domain.ProviderMidtrans: &testWebhookAdapter{
			verifyFn: func(context.Context, provider.WebhookRequest) error {
				return nil
			},
			parseFn: func(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
				return provider.WebhookEvent{}, errors.New("parse failed")
			},
		},
	})

	result, err := svc.Process(context.Background(), Input{
		Provider: domain.ProviderMidtrans,
		Request: provider.WebhookRequest{
			Headers: http.Header{"X-Test": []string{"one"}},
			Body:    []byte(`{"bad":"json"}`),
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusAccepted)
	}
	if got := result.Body["warning"]; got == nil {
		t.Fatal("warning expected, got nil")
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].ProcessingStatus != "parse_failed" {
		t.Fatalf("ProcessingStatus = %q, want parse_failed", recorder.events[0].ProcessingStatus)
	}
	if len(forwarder.inbounds) != 0 {
		t.Fatalf("Forward calls = %d, want 0", len(forwarder.inbounds))
	}
}

func TestProcessPassesThroughWithoutHandler(t *testing.T) {
	t.Parallel()

	recorder := &stubWebhookRecorder{}
	forwarder := &stubForwarder{}
	svc := New(recorder, forwarder, nil)

	result, err := svc.Process(context.Background(), Input{
		Provider: domain.ProviderXendit,
		Request: provider.WebhookRequest{
			Headers: http.Header{"X-Test": []string{"one"}},
			Body:    []byte(`{"event":"payment_session.created"}`),
		},
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusAccepted)
	}
	if got := result.Body["status"]; got != "accepted" {
		t.Fatalf("status = %v, want accepted", got)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("RecordWebhookEvent calls = %d, want 1", len(recorder.events))
	}
	if recorder.events[0].EventType != "unknown" {
		t.Fatalf("EventType = %q, want unknown", recorder.events[0].EventType)
	}
	if len(forwarder.inbounds) != 1 {
		t.Fatalf("Forward calls = %d, want 1", len(forwarder.inbounds))
	}
}

type stubWebhookRecorder struct {
	intents       map[string]domain.PaymentIntent
	events        []domain.WebhookEvent
	webhookEvents map[string]domain.WebhookEvent
}

func (s *stubWebhookRecorder) RecordWebhookEvent(_ context.Context, event domain.WebhookEvent) (string, error) {
	s.events = append(s.events, event)
	return "webhook-123", nil
}

func (s *stubWebhookRecorder) GetWebhookEventByProviderEventID(_ context.Context, providerCode domain.ProviderCode, providerEventID string) (domain.WebhookEvent, error) {
	if s.webhookEvents == nil {
		return domain.WebhookEvent{}, sql.ErrNoRows
	}
	event, ok := s.webhookEvents[webhookEventLookupKey(providerCode, providerEventID)]
	if !ok {
		return domain.WebhookEvent{}, sql.ErrNoRows
	}
	return event, nil
}

func (s *stubWebhookRecorder) GetPaymentIntentByExternalRef(_ context.Context, externalRef string) (domain.PaymentIntent, error) {
	if s.intents == nil {
		return domain.PaymentIntent{}, sql.ErrNoRows
	}
	intent, ok := s.intents[externalRef]
	if !ok {
		return domain.PaymentIntent{}, sql.ErrNoRows
	}
	return intent, nil
}

func (s *stubWebhookRecorder) UpsertPaymentIntent(_ context.Context, intent domain.PaymentIntent) (string, error) {
	if s.intents == nil {
		s.intents = make(map[string]domain.PaymentIntent)
	}
	s.intents[intent.ExternalRef] = intent
	return intent.ID, nil
}

type stubForwarder struct {
	inbounds []forwarding.InboundWebhook
}

func (s *stubForwarder) Forward(_ context.Context, inbound forwarding.InboundWebhook) error {
	s.inbounds = append(s.inbounds, inbound)
	return nil
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

func webhookEventLookupKey(providerCode domain.ProviderCode, providerEventID string) string {
	return string(providerCode) + ":" + providerEventID
}
