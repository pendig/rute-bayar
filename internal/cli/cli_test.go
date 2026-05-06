package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
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

func TestWebhookForwardCLI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")

	runCLI := func(args ...string) (string, error) {
		var stdout bytes.Buffer
		err := ExecuteWithIO(ctx, args, &stdout, &stdout)
		return stdout.String(), err
	}

	addOutput, err := runCLI(
		"webhook", "forward", "add",
		"--provider", "midtrans",
		"--name", "orders-hook",
		"--url", "https://example.test/hook",
		"--header", "X-Token=abc",
		"--event-filter", "event=payment",
		"--db", dbPath,
		"--retry-max-attempts", "4",
		"--retry-timeout", "15s",
		"--retry-backoff", "3s",
	)
	if err != nil {
		t.Fatalf("webhook forward add returned error: %v", err)
	}
	match := regexp.MustCompile(`(?m)^id:\s+([^\s]+)$`).FindStringSubmatch(addOutput)
	if len(match) != 2 {
		t.Fatalf("unexpected add output, missing id: %q", addOutput)
	}
	targetID := match[1]

	listOutput, err := runCLI("webhook", "forward", "list", "--provider", "midtrans", "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward list returned error: %v", err)
	}
	if !strings.Contains(listOutput, targetID) || !strings.Contains(listOutput, "orders-hook") {
		t.Fatalf("list output does not include target: %q", listOutput)
	}

	_, err = runCLI(
		"webhook", "forward", "update", targetID,
		"--name", "orders-hook-v2",
		"--url", "https://example.test/hook-v2",
		"--enabled", "false",
		"--db", dbPath,
	)
	if err != nil {
		t.Fatalf("webhook forward update returned error: %v", err)
	}

	disabledListOutput, err := runCLI("webhook", "forward", "list", "--provider", "midtrans", "--all", "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward list --all returned error: %v", err)
	}
	if !strings.Contains(disabledListOutput, "orders-hook-v2") || !strings.Contains(disabledListOutput, "false") {
		t.Fatalf("list --all output does not show updated target: %q", disabledListOutput)
	}

	removeOutput, err := runCLI("webhook", "forward", "remove", targetID, "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward remove returned error: %v", err)
	}
	if !strings.Contains(removeOutput, targetID) {
		t.Fatalf("remove output missing target id: %q", removeOutput)
	}

	afterRemoveOutput, err := runCLI("webhook", "forward", "list", "--provider", "midtrans", "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward list after remove returned error: %v", err)
	}
	if !strings.Contains(afterRemoveOutput, "no forwarding targets found") {
		t.Fatalf("after remove output expected no targets, got: %q", afterRemoveOutput)
	}
}

func TestWebhookForwardCLIUpdateRejectsZeroRetryMaxAttempts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")
	runCLI := func(args ...string) (string, error) {
		var stdout bytes.Buffer
		err := ExecuteWithIO(ctx, args, &stdout, &stdout)
		return stdout.String(), err
	}

	addOutput, err := runCLI(
		"webhook", "forward", "add",
		"--provider", "xendit",
		"--name", "zero-flag-hook",
		"--url", "https://example.test/hook",
		"--db", dbPath,
	)
	if err != nil {
		t.Fatalf("webhook forward add returned error: %v", err)
	}
	targetID := regexp.MustCompile(`(?m)^id:\s+([^\s]+)$`).FindStringSubmatch(addOutput)
	if len(targetID) != 2 {
		t.Fatalf("add output does not include id: %q", addOutput)
	}

	_, err = runCLI(
		"webhook", "forward", "update", targetID[1],
		"--retry-max-attempts", "0",
		"--db", dbPath,
	)
	if err == nil {
		t.Fatal("webhook forward update should reject retry-max-attempts=0")
	}
	if !strings.Contains(err.Error(), "must be greater than zero") {
		t.Fatalf("unexpected update error: %v", err)
	}
}
