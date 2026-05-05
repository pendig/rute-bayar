package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
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
