package midtrans

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
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

func TestCreatePaymentBankTransfer(t *testing.T) {
	t.Parallel()

	var receivedAuth string
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/charge" {
			t.Fatalf("request path = %q, want /v2/charge", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status_code":"201",
			"status_message":"Success, Bank Transfer transaction is created",
			"transaction_id":"tx-123",
			"order_id":"order-123",
			"payment_type":"bank_transfer",
			"transaction_status":"pending",
			"fraud_status":"accept",
			"va_numbers":[{"bank":"bca","va_number":"1234567890"}],
			"expiry_time":"2026-05-05 18:00:00 +0700"
		}`))
	}))
	defer server.Close()

	adapter := New(WithServerKey("server_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef:  "order-123",
		Amount:       15000,
		Method:       "bank_transfer",
		Channel:      "bca",
		CustomerName: "Test User",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if result.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want pending", result.Status)
	}
	if result.VANumber != "1234567890" {
		t.Fatalf("VANumber = %q, want 1234567890", result.VANumber)
	}
	if result.TransactionID != "tx-123" {
		t.Fatalf("TransactionID = %q, want tx-123", result.TransactionID)
	}
	if result.OrderID != "order-123" {
		t.Fatalf("OrderID = %q, want order-123", result.OrderID)
	}
	if result.PaymentType != "bank_transfer" {
		t.Fatalf("PaymentType = %q, want bank_transfer", result.PaymentType)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("server_key:"))
	if receivedAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", receivedAuth, wantAuth)
	}
	if got := receivedBody["payment_type"]; got != "bank_transfer" {
		t.Fatalf("payment_type = %v, want bank_transfer", got)
	}
}

func TestCreatePaymentRequiresBankTransferFields(t *testing.T) {
	t.Parallel()

	adapter := New(WithServerKey("server_key"))
	if _, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef: "order-123",
		Amount:      15000,
		Method:      "bank_transfer",
	}); err == nil {
		t.Fatal("CreatePayment returned nil error without bank code")
	}
}
