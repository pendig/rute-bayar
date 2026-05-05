package midtrans

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
)

func TestTestAuthSendsBasicAuth(t *testing.T) {
	t.Parallel()

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/rute-bayar-auth-test/status" {
			t.Fatalf("request path = %q, want /v2/rute-bayar-auth-test/status", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status_code":"404","status_message":"Transaction doesn't exist."}`))
	}))
	defer server.Close()

	adapter := New(WithServerKey("server_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if result.StatusCode != "404" {
		t.Fatalf("StatusCode = %q, want 404", result.StatusCode)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("server_key:"))
	if authHeader != want {
		t.Fatalf("Authorization = %q, want %q", authHeader, want)
	}
}

func TestTestAuthRequiresServerKey(t *testing.T) {
	t.Parallel()

	adapter := New()
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error without server key")
	}
}

func TestTestAuthRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status_code":"401"}`))
	}))
	defer server.Close()

	adapter := New(WithServerKey("bad_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for unauthorized response")
	}
}

func TestTestAuthRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	adapter := New(WithServerKey("server_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for malformed JSON")
	}
}

func TestBaseURLForEnvironment(t *testing.T) {
	t.Parallel()

	if got := BaseURLForEnvironment(domain.EnvironmentSandbox); got != "https://api.sandbox.midtrans.com" {
		t.Fatalf("sandbox base URL = %q", got)
	}
	if got := BaseURLForEnvironment(domain.EnvironmentProduction); got != "https://api.midtrans.com" {
		t.Fatalf("production base URL = %q", got)
	}
}

func TestMapTransactionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		transactionStatus string
		fraudStatus       string
		want              domain.PaymentStatus
	}{
		{name: "pending", transactionStatus: "pending", want: domain.PaymentStatusPending},
		{name: "settlement", transactionStatus: "settlement", want: domain.PaymentStatusSettled},
		{name: "capture accept", transactionStatus: "capture", fraudStatus: "accept", want: domain.PaymentStatusCaptured},
		{name: "capture challenge", transactionStatus: "capture", fraudStatus: "challenge", want: domain.PaymentStatusPending},
		{name: "deny", transactionStatus: "deny", want: domain.PaymentStatusFailed},
		{name: "failure", transactionStatus: "failure", want: domain.PaymentStatusFailed},
		{name: "cancel", transactionStatus: "cancel", want: domain.PaymentStatusCancelled},
		{name: "expire", transactionStatus: "expire", want: domain.PaymentStatusExpired},
		{name: "refund", transactionStatus: "refund", want: domain.PaymentStatusRefunded},
		{name: "partial refund", transactionStatus: "partial_refund", want: domain.PaymentStatusPartialRefunded},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MapTransactionStatus(tt.transactionStatus, tt.fraudStatus); got != tt.want {
				t.Fatalf("MapTransactionStatus(%q, %q) = %q, want %q", tt.transactionStatus, tt.fraudStatus, got, tt.want)
			}
		})
	}
}
