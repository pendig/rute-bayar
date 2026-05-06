package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
)

func TestProviderAccountUpsertListAndGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	credentialJSON, _ := json.Marshal(map[string]string{"secret_key": "one"})
	firstID, err := store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox",
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("first UpsertProviderAccount returned error: %v", err)
	}

	credentialJSON, _ = json.Marshal(map[string]string{"secret_key": "two"})
	secondID, err := store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox Updated",
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("second UpsertProviderAccount returned error: %v", err)
	}
	if secondID != firstID {
		t.Fatalf("upsert changed account id: first %q second %q", firstID, secondID)
	}

	accounts, err := store.ListProviderAccounts(ctx)
	if err != nil {
		t.Fatalf("ListProviderAccounts returned error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListProviderAccounts length = %d, want 1", len(accounts))
	}
	if accounts[0].DisplayName != "Xendit Sandbox Updated" {
		t.Fatalf("DisplayName = %q, want updated value", accounts[0].DisplayName)
	}

	account, err := store.GetProviderAccount(ctx, domain.ProviderXendit, domain.EnvironmentSandbox)
	if err != nil {
		t.Fatalf("GetProviderAccount returned error: %v", err)
	}
	if account.ID != firstID {
		t.Fatalf("GetProviderAccount ID = %q, want %q", account.ID, firstID)
	}
}

func TestGetProviderAccountNotConfiguredSentinel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.GetProviderAccount(ctx, domain.ProviderXendit, domain.EnvironmentSandbox)
	if !errors.Is(err, ErrProviderAccountNotConfigured) {
		t.Fatalf("GetProviderAccount error = %v, want ErrProviderAccountNotConfigured", err)
	}
}

func TestPaymentIntentAttemptAndStatusCheckStorage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-001",
		ProviderCode: domain.ProviderMidtrans,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.ID != intentID {
		t.Fatalf("GetPaymentIntentByExternalRef ID = %q, want %q", intent.ID, intentID)
	}

	attemptID, err := store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderMidtrans,
		RequestJSON:       []byte(`{"test":true}`),
		ResponseJSON:      []byte(`{"status":"pending"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "order-123",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}
	if attemptID == "" {
		t.Fatal("RecordPaymentAttempt returned empty id")
	}

	latestAttempt, err := store.GetLatestPaymentAttemptByIntent(ctx, intentID, domain.ProviderMidtrans)
	if err != nil {
		t.Fatalf("GetLatestPaymentAttemptByIntent returned error: %v", err)
	}
	if latestAttempt.ProviderReference != "order-123" {
		t.Fatalf("latest attempt reference = %q, want order-123", latestAttempt.ProviderReference)
	}

	checkID, err := store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderMidtrans,
		RequestJSON:       []byte(`{"order_id":"rb-001"}`),
		ResponseJSON:      []byte(`{"transaction_status":"pending"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "order-123",
	})
	if err != nil {
		t.Fatalf("RecordPaymentStatusCheck returned error: %v", err)
	}
	if checkID == "" {
		t.Fatal("RecordPaymentStatusCheck returned empty id")
	}
}

func TestRefundStorageLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-refund-001",
		ProviderCode: domain.ProviderXendit,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusSettled,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}

	refundID, err := store.RecordRefund(ctx, domain.Refund{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderXendit,
		Amount:            5000,
		Status:            domain.PaymentStatusRefunded,
		RequestJSON:       []byte(`{"payment_request_id":"pr-001"}`),
		ResponseJSON:      []byte(`{"status":"SUCCEEDED"}`),
		ProviderReference: "ps-001",
	})
	if err != nil {
		t.Fatalf("RecordRefund returned error: %v", err)
	}
	if refundID == "" {
		t.Fatal("RecordRefund returned empty id")
	}

	refund, err := store.GetLatestRefundByIntent(ctx, intentID, domain.ProviderXendit)
	if err != nil {
		t.Fatalf("GetLatestRefundByIntent returned error: %v", err)
	}
	if refund.ProviderReference != "ps-001" {
		t.Fatalf("ProviderReference = %q, want ps-001", refund.ProviderReference)
	}
	if refund.Status != domain.PaymentStatusRefunded {
		t.Fatalf("Status = %q, want refunded", refund.Status)
	}
	if string(refund.RequestJSON) != `{"payment_request_id":"pr-001"}` {
		t.Fatalf("RequestJSON = %s, want original request", refund.RequestJSON)
	}
	if string(refund.ResponseJSON) != `{"status":"SUCCEEDED"}` {
		t.Fatalf("ResponseJSON = %s, want original response", refund.ResponseJSON)
	}
}

func TestWebhookEventStorageLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	receivedAt := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	processedAt := time.Date(2026, 5, 6, 9, 0, 1, 0, time.UTC)

	eventID, err := store.RecordWebhookEvent(ctx, domain.WebhookEvent{
		ProviderCode:     domain.ProviderXendit,
		ProviderEventID:  "evt-123",
		EventType:        "payment.capture",
		SignatureValid:   true,
		PayloadJSON:      []byte(`{"event":"payment.capture"}`),
		HeadersJSON:      []byte(`{"X-Test":["one"]}`),
		ReceivedAt:       receivedAt,
		ProcessedAt:      &processedAt,
		ProcessingStatus: "processed",
	})
	if err != nil {
		t.Fatalf("RecordWebhookEvent returned error: %v", err)
	}
	if eventID == "" {
		t.Fatal("RecordWebhookEvent returned empty id")
	}

	event, err := store.GetWebhookEventByProviderEventID(ctx, domain.ProviderXendit, "evt-123")
	if err != nil {
		t.Fatalf("GetWebhookEventByProviderEventID returned error: %v", err)
	}
	if event.ProviderEventID != "evt-123" {
		t.Fatalf("ProviderEventID = %q, want evt-123", event.ProviderEventID)
	}
	if !event.SignatureValid {
		t.Fatal("SignatureValid = false, want true")
	}
	if event.ProcessedAt == nil {
		t.Fatal("ProcessedAt is nil, want populated value")
	}
	if !event.ProcessedAt.Equal(processedAt) {
		t.Fatalf("ProcessedAt = %v, want %v", event.ProcessedAt, processedAt)
	}

	if _, err := store.GetWebhookEventByProviderEventID(ctx, domain.ProviderMidtrans, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWebhookEventByProviderEventID missing error = %v, want sql.ErrNoRows", err)
	}
}

func TestForwardingTargetLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	targetID, err := store.AddForwardingTarget(ctx, forwarding.Target{
		Name:     "xendit-webhook",
		Provider: domain.ProviderXendit,
		URL:      "https://example.test/callback",
		Headers: http.Header{
			"Authorization": []string{"Bearer token"},
			"X-Trace":       []string{"first"},
		},
		EventFilter: map[string]string{
			"event": "payment_session.created",
			"env":   "sandbox",
		},
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: 5,
			Timeout:     8 * time.Second,
			Backoff:     1 * time.Second,
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddForwardingTarget returned error: %v", err)
	}
	if targetID == "" {
		t.Fatal("AddForwardingTarget returned empty id")
	}

	targets, err := store.ListForwardingTargets(ctx, domain.ProviderXendit)
	if err != nil {
		t.Fatalf("ListForwardingTargets returned error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("ListForwardingTargets length = %d, want 1", len(targets))
	}
	if targets[0].Headers.Get("Authorization") != "Bearer token" {
		t.Fatalf("stored header Authorization = %q, want token", targets[0].Headers.Get("Authorization"))
	}
	if got := targets[0].EventFilter["event"]; got != "payment_session.created" {
		t.Fatalf("stored event filter event = %q, want payment_session.created", got)
	}

	target, err := store.GetForwardingTarget(ctx, targetID)
	if err != nil {
		t.Fatalf("GetForwardingTarget returned error: %v", err)
	}
	if target.Name != "xendit-webhook" {
		t.Fatalf("GetForwardingTarget Name = %q, want xendit-webhook", target.Name)
	}

	updatedHeaders := http.Header{"Authorization": []string{"Bearer updated"}}
	target.Name = "xendit-webhook-v2"
	target.URL = "https://example.test/callback-v2"
	target.Headers = updatedHeaders
	target.Enabled = false
	target.RetryPolicy = forwarding.RetryPolicy{
		MaxAttempts: 7,
		Timeout:     10 * time.Second,
		Backoff:     2 * time.Second,
	}
	if err := store.UpdateForwardingTarget(ctx, target); err != nil {
		t.Fatalf("UpdateForwardingTarget returned error: %v", err)
	}

	target, err = store.GetForwardingTarget(ctx, targetID)
	if err != nil {
		t.Fatalf("GetForwardingTarget after update returned error: %v", err)
	}
	if target.Name != "xendit-webhook-v2" {
		t.Fatalf("updated Name = %q, want xendit-webhook-v2", target.Name)
	}
	if target.URL != "https://example.test/callback-v2" {
		t.Fatalf("updated URL = %q, want https://example.test/callback-v2", target.URL)
	}
	if target.Headers.Get("Authorization") != "Bearer updated" {
		t.Fatalf("updated Authorization = %q, want updated", target.Headers.Get("Authorization"))
	}
	if target.Enabled {
		t.Fatal("target should be disabled after update")
	}
	if target.RetryPolicy.MaxAttempts != 7 {
		t.Fatalf("updated retry max attempts = %d, want 7", target.RetryPolicy.MaxAttempts)
	}

	enabledTargets, err := store.ListEnabledTargets(ctx, domain.ProviderXendit)
	if err != nil {
		t.Fatalf("ListEnabledTargets returned error: %v", err)
	}
	if len(enabledTargets) != 0 {
		t.Fatalf("ListEnabledTargets length = %d, want 0", len(enabledTargets))
	}

	if err := store.DeleteForwardingTarget(ctx, targetID); err != nil {
		t.Fatalf("DeleteForwardingTarget returned error: %v", err)
	}

	if _, err := store.GetForwardingTarget(ctx, targetID); err == nil {
		t.Fatal("GetForwardingTarget returned nil error after delete")
	}
}

func TestGetWebhookEventByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	eventID, err := store.RecordWebhookEvent(ctx, domain.WebhookEvent{
		ProviderCode:     domain.ProviderXendit,
		ProviderEventID:  "evt-lookup-001",
		EventType:        "payment.capture",
		SignatureValid:   true,
		PayloadJSON:      []byte(`{"event":"payment.capture"}`),
		HeadersJSON:      []byte(`{"X-Test":["one"]}`),
		ProcessingStatus: "processed",
	})
	if err != nil {
		t.Fatalf("RecordWebhookEvent returned error: %v", err)
	}
	if eventID == "" {
		t.Fatal("RecordWebhookEvent returned empty id")
	}

	event, err := store.GetWebhookEventByID(ctx, eventID)
	if err != nil {
		t.Fatalf("GetWebhookEventByID returned error: %v", err)
	}
	if event.ID != eventID {
		t.Fatalf("event.ID = %q, want %q", event.ID, eventID)
	}
	if event.ProviderCode != domain.ProviderXendit {
		t.Fatalf("ProviderCode = %q, want %q", event.ProviderCode, domain.ProviderXendit)
	}
	if event.ProviderEventID != "evt-lookup-001" {
		t.Fatalf("ProviderEventID = %q, want evt-lookup-001", event.ProviderEventID)
	}

	if _, err := store.GetWebhookEventByID(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWebhookEventByID missing error = %v, want sql.ErrNoRows", err)
	}
}

func TestHeadersJSONCodecSupportsSingleAndMultiValues(t *testing.T) {
	t.Parallel()

	rawSingle := `{"Authorization":"Bearer token","X-Trace":["t1","t2"]}`
	headers := headersFromJSON(rawSingle)
	if len(headers["Authorization"]) != 1 || headers.Get("Authorization") != "Bearer token" {
		t.Fatalf("headersFromJSON single value = %q, want Bearer token", headers.Get("Authorization"))
	}
	if !reflect.DeepEqual(headers["X-Trace"], []string{"t1", "t2"}) {
		t.Fatalf("headersFromJSON multi value = %#v, want %#v", headers["X-Trace"], []string{"t1", "t2"})
	}

	multiple := http.Header{
		"Authorization": {"Bearer token"},
		"X-Trace":       {"t1", "t2"},
	}
	raw := headersToJSON(multiple)
	var decoded map[string][]string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("headersToJSON returned invalid json: %v", err)
	}
	if !reflect.DeepEqual(decoded["Authorization"], []string{"Bearer token"}) {
		t.Fatalf("headersToJSON authorization = %#v, want %#v", decoded["Authorization"], []string{"Bearer token"})
	}
	if !reflect.DeepEqual(decoded["X-Trace"], []string{"t1", "t2"}) {
		t.Fatalf("headersToJSON trace = %#v, want %#v", decoded["X-Trace"], []string{"t1", "t2"})
	}
}
