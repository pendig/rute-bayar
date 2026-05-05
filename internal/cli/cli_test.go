package cli

import (
	"context"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "short secret", in: "secret", want: "********"},
		{name: "long secret", in: "xnd_development_abcdef", want: "xnd_**************cdef"},
		{name: "trims whitespace", in: " 123456789 ", want: "1234*6789"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := maskSecret(tt.in); got != tt.want {
				t.Fatalf("maskSecret(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSecretKeyFromCredential(t *testing.T) {
	t.Parallel()

	secret, err := secretKeyFromCredential([]byte(`{"secret_key":" xnd_test_key "}`))
	if err != nil {
		t.Fatalf("secretKeyFromCredential returned error: %v", err)
	}
	if secret != "xnd_test_key" {
		t.Fatalf("secretKeyFromCredential = %q, want xnd_test_key", secret)
	}
}

func TestSecretKeyFromCredentialRequiresSecret(t *testing.T) {
	t.Parallel()

	if _, err := secretKeyFromCredential([]byte(`{"secret_key":""}`)); err == nil {
		t.Fatal("secretKeyFromCredential returned nil error for missing secret")
	}
}

func TestMidtransCredentialFromJSON(t *testing.T) {
	t.Parallel()

	credential, err := midtransCredentialFromJSON([]byte(`{
		"merchant_id":" merchant ",
		"client_key":" client ",
		"server_key":" server "
	}`))
	if err != nil {
		t.Fatalf("midtransCredentialFromJSON returned error: %v", err)
	}
	if credential.MerchantID != "merchant" {
		t.Fatalf("MerchantID = %q, want merchant", credential.MerchantID)
	}
	if credential.ClientKey != "client" {
		t.Fatalf("ClientKey = %q, want client", credential.ClientKey)
	}
	if credential.ServerKey != "server" {
		t.Fatalf("ServerKey = %q, want server", credential.ServerKey)
	}
}

func TestMidtransCredentialFromJSONRequiresFields(t *testing.T) {
	t.Parallel()

	if _, err := midtransCredentialFromJSON([]byte(`{"merchant_id":"merchant","client_key":"client"}`)); err == nil {
		t.Fatal("midtransCredentialFromJSON returned nil error for missing server key")
	}
}

func TestValidateEnvironment(t *testing.T) {
	t.Parallel()

	if err := validateEnvironment("sandbox"); err != nil {
		t.Fatalf("validateEnvironment(sandbox) returned error: %v", err)
	}
	if err := validateEnvironment("production"); err != nil {
		t.Fatalf("validateEnvironment(production) returned error: %v", err)
	}
	if err := validateEnvironment("staging"); err == nil {
		t.Fatal("validateEnvironment(staging) returned nil error")
	}
}

func TestBuildWebhookHandlersAllowsUnconfiguredProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	handlers, err := buildWebhookHandlers(ctx, store, domain.EnvironmentSandbox)
	if err != nil {
		t.Fatalf("buildWebhookHandlers returned error: %v", err)
	}
	if len(handlers) != 0 {
		t.Fatalf("handlers length = %d, want 0", len(handlers))
	}
}

func TestBuildWebhookHandlersReturnsMalformedCredentialError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderMidtrans,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Midtrans Sandbox",
		CredentialJSON: []byte(`{"merchant_id":"merchant","client_key":"client"}`),
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	if _, err := buildWebhookHandlers(ctx, store, domain.EnvironmentSandbox); err == nil {
		t.Fatal("buildWebhookHandlers returned nil error for malformed credential")
	}
}
