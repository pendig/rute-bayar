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

