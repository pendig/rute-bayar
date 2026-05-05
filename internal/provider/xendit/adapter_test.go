package xendit

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
		if r.URL.Path != "/balance" {
			t.Fatalf("request path = %q, want /balance", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"balance":1000}`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("secret_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if info.Balance == nil || *info.Balance != 1000 {
		t.Fatalf("Balance = %v, want 1000", info.Balance)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("secret_key:"))
	if authHeader != want {
		t.Fatalf("Authorization = %q, want %q", authHeader, want)
	}
}

func TestTestAuthRequiresSecretKey(t *testing.T) {
	t.Parallel()

	adapter := New()
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error without secret key")
	}
}

func TestTestAuthRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_code":"INVALID_API_KEY"}`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("bad_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for unauthorized response")
	}
}

func TestTestAuthAllowsForbiddenBalancePermission(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error_code":"REQUEST_FORBIDDEN_ERROR"}`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("money_in_only_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error for forbidden balance permission: %v", err)
	}
	if info.PermissionWarning == "" {
		t.Fatal("PermissionWarning is empty for forbidden balance permission")
	}
}

func TestTestAuthRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("secret_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for malformed JSON")
	}
}

func TestGetPaymentStatusMapsActiveSession(t *testing.T) {
	t.Parallel()

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"ps_123",
			"reference_id":"rb-001",
			"mode":"PAYMENT_LINK",
			"status":"ACTIVE",
			"payment_link_url":"https://example.com/pay"
		}`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("secret_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := adapter.GetPaymentStatus(context.Background(), "ps_123")
	if err != nil {
		t.Fatalf("GetPaymentStatus returned error: %v", err)
	}
	if requestedPath != "/sessions/ps_123" {
		t.Fatalf("requested path = %q, want /sessions/ps_123", requestedPath)
	}
	if result.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want pending", result.Status)
	}
	if result.ProviderReference != "ps_123" {
		t.Fatalf("ProviderReference = %q, want ps_123", result.ProviderReference)
	}
	if result.OrderID != "rb-001" {
		t.Fatalf("OrderID = %q, want rb-001", result.OrderID)
	}
}
