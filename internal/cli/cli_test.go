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
	"sync/atomic"
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

func TestResolveAPIOperation_MidtransUsesGeneratedAliasAndManualFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		method    string
		path      string
	}{
		{name: "generated snap", operation: "snap-v1-transaction", method: http.MethodPost, path: "/snap/v1/transactions"},
		{name: "generated status", operation: "status", method: http.MethodGet, path: "/v2/{order_id}/status"},
		{name: "generated approve", operation: "approve", method: http.MethodPost, path: "/v2/{order_id}/approve"},
		{name: "generated deny", operation: "deny", method: http.MethodPost, path: "/v2/{order_id}/deny"},
		{name: "generated cancel", operation: "cancel", method: http.MethodPost, path: "/v2/{order_id}/cancel"},
		{name: "generated expire", operation: "expire", method: http.MethodPost, path: "/v2/{order_id}/expire"},
		{name: "generated refund", operation: "refund", method: http.MethodPost, path: "/v2/{order_id}/refund"},
		{name: "manual check-status", operation: "check-status", method: http.MethodGet, path: "/v2/{order_id}/status"},
		{name: "manual auth-test", operation: "auth-test", method: http.MethodGet, path: "/v2/rute-bayar-auth-test/status"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			alias, ok := resolveAPIOperation(domain.ProviderMidtrans, tc.operation)
			if !ok {
				t.Fatalf("operation %q should be available", tc.operation)
			}
			if alias.method != tc.method {
				t.Fatalf("operation %q method = %q, want %q", tc.operation, alias.method, tc.method)
			}
			if alias.path != tc.path {
				t.Fatalf("operation %q path = %q, want %q", tc.operation, alias.path, tc.path)
			}
		})
	}
}

func TestResolveAPIOperation_XenditUsesGeneratedAliasAndManualFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		method    string
		path      string
	}{
		{name: "generated payment-requests", operation: "payment-requests", method: http.MethodGet, path: "/payment_requests"},
		{name: "generated payment-request", operation: "get-payment-request", method: http.MethodGet, path: "/payment_requests"},
		{name: "manual auth-balance", operation: "auth-balance", method: http.MethodGet, path: "/balance"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			alias, ok := resolveAPIOperation(domain.ProviderXendit, tc.operation)
			if !ok {
				t.Fatalf("operation %q should be available", tc.operation)
			}
			if alias.method != tc.method {
				t.Fatalf("operation %q method = %q, want %q", tc.operation, alias.method, tc.method)
			}
			if alias.path != tc.path {
				t.Fatalf("operation %q path = %q, want %q", tc.operation, alias.path, tc.path)
			}
		})
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

func TestWebhookForwardCLIUpdateRejectsNegativeRetryMaxAttempts(t *testing.T) {
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
		"--retry-max-attempts", "-1",
		"--db", dbPath,
	)
	if err == nil {
		t.Fatal("webhook forward update should reject retry-max-attempts=-1")
	}
	if !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("unexpected update error: %v", err)
	}
}

func TestWebhookReplayCLI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")
	runCLI := func(args ...string) (string, error) {
		var stdout bytes.Buffer
		err := ExecuteWithIO(ctx, args, &stdout, &stdout)
		return stdout.String(), err
	}

	forwardCount := int32(0)
	blockCount := int32(0)
	createdServer := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "invalid method", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		atomic.AddInt32(&forwardCount, 1)
	}))
	if createdServer != nil {
		defer createdServer.Close()
	}
	failedServer := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "invalid method", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		atomic.AddInt32(&blockCount, 1)
	}))
	if failedServer != nil {
		defer failedServer.Close()
	}

	if createdServer == nil || failedServer == nil {
		t.Skip("skip replay CLI test: webhook replay requires local HTTP server")
	}

	_, err := runCLI(
		"webhook", "forward", "add",
		"--provider", "midtrans",
		"--name", "midtrans-created",
		"--url", createdServer.URL,
		"--event-filter", "event=payment_session.created",
		"--db", dbPath,
	)
	if err != nil {
		t.Fatalf("webhook forward add for created target returned error: %v", err)
	}

	_, err = runCLI(
		"webhook", "forward", "add",
		"--provider", "midtrans",
		"--name", "midtrans-failed",
		"--url", failedServer.URL,
		"--event-filter", "event=payment_session.failed",
		"--db", dbPath,
	)
	if err != nil {
		t.Fatalf("webhook forward add for failed target returned error: %v", err)
	}

	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	eventID, err := store.RecordWebhookEvent(ctx, domain.WebhookEvent{
		ProviderCode:    domain.ProviderMidtrans,
		ProviderEventID: "replay-event-001",
		EventType:       "payment_session.created",
		SignatureValid:  true,
		PayloadJSON:     []byte(`{"event":"payment_session.created","reference_id":"rb-001"}`),
		HeadersJSON:     []byte(`{"X-Trace":["one"]}`),
	})
	if err != nil {
		t.Fatalf("RecordWebhookEvent returned error: %v", err)
	}

	_, err = runCLI("webhook", "replay", "--event-id", eventID, "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook replay returned error: %v", err)
	}

	if got := atomic.LoadInt32(&forwardCount); got != 1 {
		t.Fatalf("forwardCount = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&blockCount); got != 0 {
		t.Fatalf("blockCount = %d, want 0", got)
	}
}

func TestWebhookForwardAttemptsCLI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rute-bayar.sqlite3")
	runCLI := func(args ...string) (string, error) {
		var stdout bytes.Buffer
		err := ExecuteWithIO(ctx, args, &stdout, &stdout)
		return stdout.String(), err
	}

	var acceptRetry int32
	var requestCount int32
	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		if atomic.LoadInt32(&acceptRetry) == 0 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	if server == nil {
		t.Skip("skip forwarding attempt CLI test: local HTTP server is unavailable")
	}
	defer server.Close()

	addOutput, err := runCLI(
		"webhook", "forward", "add",
		"--provider", "xendit",
		"--name", "diagnostic-hook",
		"--url", server.URL,
		"--event-filter", "event=payment_session.created",
		"--retry-max-attempts", "1",
		"--db", dbPath,
	)
	if err != nil {
		t.Fatalf("webhook forward add returned error: %v", err)
	}
	targetMatch := regexp.MustCompile(`(?m)^id:\s+([^\s]+)$`).FindStringSubmatch(addOutput)
	if len(targetMatch) != 2 {
		t.Fatalf("unexpected add output, missing target id: %q", addOutput)
	}

	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	eventID, err := store.RecordWebhookEvent(ctx, domain.WebhookEvent{
		ProviderCode:     domain.ProviderXendit,
		ProviderEventID:  "evt-forward-attempt-cli",
		EventType:        "payment_session.created",
		SignatureValid:   true,
		PayloadJSON:      []byte(`{"event":"payment_session.created","status":"ACTIVE","reference_id":"rb-attempt-cli"}`),
		HeadersJSON:      []byte(`{"X-Trace":["attempt-cli"]}`),
		ProcessingStatus: "processed",
	})
	if err != nil {
		t.Fatalf("RecordWebhookEvent returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	_, err = runCLI("webhook", "replay", "--event-id", eventID, "--provider", "xendit", "--db", dbPath)
	if err == nil {
		t.Fatal("webhook replay should fail while target returns 500")
	}

	listOutput, err := runCLI("webhook", "forward", "attempts", "list", "--status", "failed", "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward attempts list returned error: %v", err)
	}
	attemptMatch := regexp.MustCompile(`(?m)^(fwd_attempt_[^\s]+)`).FindStringSubmatch(listOutput)
	if len(attemptMatch) != 2 {
		t.Fatalf("attempt list output missing attempt id: %q", listOutput)
	}
	attemptID := attemptMatch[1]

	showOutput, err := runCLI("webhook", "forward", "attempts", "show", attemptID, "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward attempts show returned error: %v", err)
	}
	if !strings.Contains(showOutput, "response_json:") || !strings.Contains(showOutput, "temporary failure") {
		t.Fatalf("show output missing raw response detail: %q", showOutput)
	}

	atomic.StoreInt32(&acceptRetry, 1)
	retryOutput, err := runCLI("webhook", "forward", "attempts", "retry", attemptID, "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward attempts retry returned error: %v", err)
	}
	if !strings.Contains(retryOutput, "forwarding attempt retried") {
		t.Fatalf("unexpected retry output: %q", retryOutput)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("requestCount = %d, want 2", got)
	}

	successOutput, err := runCLI("webhook", "forward", "attempts", "list", "--status", "success", "--db", dbPath)
	if err != nil {
		t.Fatalf("webhook forward attempts list success returned error: %v", err)
	}
	if !strings.Contains(successOutput, "success") {
		t.Fatalf("success list output missing success attempt: %q", successOutput)
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

func TestPayRefundXenditRecordsRefund(t *testing.T) {
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

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-xnd-refund",
		ProviderCode: domain.ProviderXendit,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusSettled,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}
	_, err = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderXendit,
		RequestJSON:       []byte(`{"request":"create"}`),
		ResponseJSON:      []byte(`{"response":"create"}`),
		Status:            domain.PaymentStatusSettled,
		ProviderReference: "ps-xnd-001",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sessions/ps-xnd-001":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"payment_session_id":"ps-xnd-001",
				"reference_id":"rb-xnd-refund",
				"payment_request_id":"pr-xnd-001",
				"status":"COMPLETED",
				"payment_link_url":"https://example.com/pay"
			}`))
		case r.Method == http.MethodPost && r.URL.Path == "/refunds":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload["payment_request_id"] != "pr-xnd-001" {
				http.Error(w, "unexpected payment_request_id", http.StatusBadRequest)
				return
			}
			if payload["reference_id"] != "rb-xnd-refund-001" {
				http.Error(w, "unexpected reference_id", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id":"rfd-001",
				"payment_request_id":"pr-xnd-001",
				"status":"SUCCEEDED",
				"reason":"REQUESTED_BY_CUSTOMER"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err = ExecuteWithIO(ctx, []string{
		"pay",
		"refund",
		"--provider",
		"xendit",
		"--reference",
		"rb-xnd-refund",
		"--refund-reference",
		"rb-xnd-refund-001",
		"--amount",
		"5000",
		"--db",
		dbPath,
		"--environment",
		"sandbox",
		"--base-url",
		server.URL,
	}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("pay refund returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "provider: xendit") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "status: refunded") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "refund_reference: rb-xnd-refund-001") {
		t.Fatalf("unexpected output: %q", output)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-refund")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusRefunded {
		t.Fatalf("intent status = %q, want refunded", intent.Status)
	}
}

func TestReconcileMidtransUpdatesIntent(t *testing.T) {
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
		ExternalRef:  "rb-mid-reconcile",
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
		RequestJSON:       []byte(`{"request":"create"}`),
		ResponseJSON:      []byte(`{"response":"create"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "order-mid-reconcile",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/order-mid-reconcile/status" {
			http.NotFound(w, r)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-mid-reconcile",
			"order_id":"order-mid-reconcile",
			"transaction_status":"settlement",
			"fraud_status":"accept",
			"payment_type":"bank_transfer"
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err = ExecuteWithIO(ctx, []string{
		"reconcile",
		"--provider",
		"midtrans",
		"--reference",
		"rb-mid-reconcile",
		"--db",
		dbPath,
		"--environment",
		"sandbox",
		"--base-url",
		server.URL,
	}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "local_status: pending") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "provider_status: settled") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(output, "updated: true") {
		t.Fatalf("unexpected output: %q", output)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-mid-reconcile")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want settled", intent.Status)
	}
}
