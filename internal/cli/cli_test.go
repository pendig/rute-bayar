package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func newHTTPTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skip HTTP server test: bind to local TCP is restricted (%v)", err)
		return nil
	}

	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

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

func TestPayStatusMidtransUpdatesIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	credentialJSON, err := json.Marshal(struct {
		MerchantID string `json:"merchant_id"`
		ClientKey  string `json:"client_key"`
		ServerKey  string `json:"server_key"`
	}{
		MerchantID: "merchant",
		ClientKey:  "client",
		ServerKey:  "server",
	})
	if err != nil {
		t.Fatalf("marshal midtrans credential: %v", err)
	}
	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderMidtrans,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Midtrans Sandbox",
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-mid-001",
		ProviderCode: domain.ProviderMidtrans,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}

	_, err = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderMidtrans,
		RequestJSON:       []byte(`{"request":"test"}`),
		ResponseJSON:      []byte(`{"response":"test"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "order-mid-001",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid method", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/v2/order-mid-001/status" {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-mid-001",
			"order_id":"order-mid-001",
			"transaction_status":"settlement",
			"fraud_status":"accept",
			"payment_type":"bank_transfer"
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err = ExecuteWithIO(ctx, []string{
		"pay",
		"status",
		"--provider",
		"midtrans",
		"--reference",
		"rb-mid-001",
		"--provider-reference",
		"order-mid-001",
		"--db",
		dbPath,
		"--environment",
		"sandbox",
		"--base-url",
		server.URL,
	}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("pay status returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "provider: midtrans") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "status: settled") {
		t.Fatalf("unexpected output: %q", output)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-mid-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want %q", intent.Status, domain.PaymentStatusSettled)
	}
}

func TestPayStatusXenditUpdatesIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	credentialJSON, err := json.Marshal(map[string]string{"secret_key": "secret"})
	if err != nil {
		t.Fatalf("marshal xendit credential: %v", err)
	}
	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox",
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	_, err = store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-xnd-001",
		ProviderCode: domain.ProviderXendit,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid method", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/sessions/ps-xnd-001" {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"ps-xnd-001",
			"reference_id":"rb-xnd-001",
			"mode":"PAYMENT_LINK",
			"status":"COMPLETED",
			"payment_link_url":"https://example.com/pay"
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err = ExecuteWithIO(ctx, []string{
		"pay",
		"status",
		"--provider",
		"xendit",
		"--reference",
		"rb-xnd-001",
		"--provider-reference",
		"ps-xnd-001",
		"--db",
		dbPath,
		"--environment",
		"sandbox",
		"--base-url",
		server.URL,
	}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("pay status returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "provider: xendit") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "status: settled") {
		t.Fatalf("unexpected output: %q", output)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want %q", intent.Status, domain.PaymentStatusSettled)
	}
}
