package midtrans

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestTestAuthSendsBasicAuth(t *testing.T) {
	t.Parallel()

	var authHeader string
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/rute-bayar-auth-test/status" {
			t.Fatalf("request path = %q, want /v2/rute-bayar-auth-test/status", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		return response(http.StatusNotFound, `{"status_code":"404","status_message":"Transaction doesn't exist."}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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

	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusUnauthorized, `{"status_code":"401"}`), nil
	})}

	adapter := New(WithServerKey("bad_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for unauthorized response")
	}
}

func TestTestAuthRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `not-json`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/charge" {
			t.Fatalf("request path = %q, want /v2/charge", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return response(http.StatusOK, `{
			"status_code":"201",
			"status_message":"Success, Bank Transfer transaction is created",
			"transaction_id":"tx-123",
			"order_id":"order-123",
			"payment_type":"bank_transfer",
			"transaction_status":"pending",
			"fraud_status":"accept",
			"va_numbers":[{"bank":"bca","va_number":"1234567890"}],
			"expiry_time":"2026-05-05 18:00:00 +0700"
		}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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

func TestCreatePaymentCreditCard(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/charge" {
			t.Fatalf("request path = %q, want /v2/charge", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return response(http.StatusOK, `{
			"status_code":"201",
			"status_message":"Success, Credit Card transaction is created",
			"transaction_id":"tx-card-123",
			"order_id":"order-card-123",
			"payment_type":"credit_card",
			"transaction_status":"pending",
			"fraud_status":"accept",
			"redirect_url":"https://api.sandbox.midtrans.com/v2/token/rba/redirect/tx-card-123"
		}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef:   "order-card-123",
		Amount:        15000,
		Method:        "card",
		CardToken:     "card-token-123",
		CustomerName:  "Test User",
		CustomerEmail: "test@example.test",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if result.PaymentType != "credit_card" {
		t.Fatalf("PaymentType = %q, want credit_card", result.PaymentType)
	}
	if result.RedirectURL == "" {
		t.Fatal("RedirectURL is empty")
	}

	if got := receivedBody["payment_type"]; got != "credit_card" {
		t.Fatalf("payment_type = %v, want credit_card", got)
	}
	creditCard, ok := receivedBody["credit_card"].(map[string]any)
	if !ok {
		t.Fatalf("credit_card = %T, want object", receivedBody["credit_card"])
	}
	if creditCard["token_id"] != "card-token-123" {
		t.Fatalf("credit_card.token_id = %v, want card-token-123", creditCard["token_id"])
	}
	if creditCard["authentication"] != true {
		t.Fatalf("credit_card.authentication = %v, want true", creditCard["authentication"])
	}
	if _, ok := receivedBody["bank_transfer"]; ok {
		t.Fatalf("bank_transfer should be omitted for credit card: %v", receivedBody["bank_transfer"])
	}
}

func TestCreatePaymentCreditCardRequiresToken(t *testing.T) {
	t.Parallel()

	adapter := New(WithServerKey("server_key"))
	if _, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef: "order-card-123",
		Amount:      15000,
		Method:      "credit_card",
	}); err == nil {
		t.Fatal("CreatePayment returned nil error without card token")
	}
}

func TestRefundPaymentPostsToOrderRefund(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("request method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v2/order-123/refund" {
			t.Fatalf("request path = %q, want /v2/order-123/refund", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return response(http.StatusOK, `{
			"status_code":"200",
			"status_message":"Success, refund transaction is created",
			"transaction_id":"tx-123",
			"order_id":"order-123",
			"transaction_status":"refund",
			"refund_key":"refund-001"
		}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.RefundPayment(context.Background(), provider.RefundRequest{
		ProviderReference: "order-123",
		ReferenceID:       "refund-001",
		Amount:            5000,
		Currency:          "IDR",
		Reason:            "requested by customer",
	})
	if err != nil {
		t.Fatalf("RefundPayment returned error: %v", err)
	}
	if result.Status != domain.PaymentStatusRefunded {
		t.Fatalf("Status = %q, want refunded", result.Status)
	}
	if result.ProviderReference != "order-123" {
		t.Fatalf("ProviderReference = %q, want order-123", result.ProviderReference)
	}
	if got := receivedBody["refund_key"]; got != "refund-001" {
		t.Fatalf("refund_key = %v, want refund-001", got)
	}
}

func TestRefundPaymentRejectsBusinessErrorStatusCode(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"status_code":"412",
			"status_message":"Transaction status cannot be updated.",
			"id":"business-error"
		}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.RefundPayment(context.Background(), provider.RefundRequest{
		ProviderReference: "order-123",
		ReferenceID:       "refund-001",
		Amount:            5000,
	})
	if err == nil {
		t.Fatal("RefundPayment returned nil error for business error status_code")
	}
	if result.Status != domain.PaymentStatusFailed {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if len(result.RawResponseJSON) == 0 {
		t.Fatal("RawResponseJSON is empty")
	}
}

func TestGetPaymentStatusBankTransfer(t *testing.T) {
	t.Parallel()

	var requestedPath string
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requestedPath = r.URL.Path
		if r.Header.Get("Authorization") == "" {
			t.Fatal("Authorization header is empty")
		}
		return response(http.StatusOK, `{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-123",
			"order_id":"order-123",
			"payment_type":"bank_transfer",
			"transaction_status":"pending",
			"fraud_status":"accept",
			"va_numbers":[{"bank":"bca","va_number":"1234567890"}],
			"expiry_time":"2026-05-05 18:00:00 +0700"
		}`), nil
	})}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.GetPaymentStatus(context.Background(), "order-123")
	if err != nil {
		t.Fatalf("GetPaymentStatus returned error: %v", err)
	}
	if requestedPath != "/v2/order-123/status" {
		t.Fatalf("requested path = %q, want /v2/order-123/status", requestedPath)
	}
	if result.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want pending", result.Status)
	}
	if result.VANumber != "1234567890" {
		t.Fatalf("VANumber = %q, want 1234567890", result.VANumber)
	}
	if result.OrderID != "order-123" {
		t.Fatalf("OrderID = %q, want order-123", result.OrderID)
	}
}

func TestVerifyWebhookAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	sum := sha512.Sum512([]byte("order-12320010000server_key"))
	notification := map[string]any{
		"order_id":      "order-123",
		"status_code":   "200",
		"gross_amount":  "10000",
		"signature_key": hex.EncodeToString(sum[:]),
	}

	payload, _ := json.Marshal(notification)
	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	}); err != nil {
		t.Fatalf("VerifyWebhook returned error: %v", err)
	}
}

func TestVerifyWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"order_id":      "order-123",
		"status_code":   "200",
		"gross_amount":  "10000",
		"signature_key": "invalid",
	})
	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	}); err == nil {
		t.Fatal("VerifyWebhook returned nil error for invalid signature")
	}
}

func TestVerifyWebhookAcceptsNumericGrossAmount(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"order_id":     "order-321",
		"status_code":  "200",
		"gross_amount": 10000,
	}
	sum := sha512.Sum512([]byte("order-32120010000server_key"))
	payload["signature_key"] = hex.EncodeToString(sum[:])

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload returned error: %v", err)
	}

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    raw,
	}); err != nil {
		t.Fatalf("VerifyWebhook returned error: %v", err)
	}
}

func TestParseWebhookMapsStatus(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"order_id":           "order-123",
		"transaction_id":     "tx-123",
		"transaction_status": "capture",
		"fraud_status":       "accept",
		"payment_type":       "bank_transfer",
	})
	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "capture:order-123:tx-123" {
		t.Fatalf("ProviderEventID = %q, want capture:order-123:tx-123", event.ProviderEventID)
	}
	if event.EventType != "capture" {
		t.Fatalf("EventType = %q, want capture", event.EventType)
	}
	if event.Status != domain.PaymentStatusCaptured {
		t.Fatalf("Status = %q, want %q", event.Status, domain.PaymentStatusCaptured)
	}
}

func TestParseWebhookFallsBackPaymentReference(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"transaction_id":     "tx-456",
		"transaction_status": "pending",
		"fraud_status":       "accept",
		"payment_type":       "bank_transfer",
	})
	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "pending:tx-456" {
		t.Fatalf("ProviderEventID = %q, want pending:tx-456", event.ProviderEventID)
	}
	if event.PaymentRef != "tx-456" {
		t.Fatalf("PaymentRef = %q, want tx-456", event.PaymentRef)
	}
	if event.EventType != "pending" {
		t.Fatalf("EventType = %q, want pending", event.EventType)
	}
}

func TestParseWebhookLeavesProviderEventIDEmptyWithoutIdentifiers(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"transaction_status": "pending",
		"fraud_status":       "accept",
		"payment_type":       "bank_transfer",
	})
	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "" {
		t.Fatalf("ProviderEventID = %q, want empty", event.ProviderEventID)
	}
	if event.PaymentRef != "" {
		t.Fatalf("PaymentRef = %q, want empty", event.PaymentRef)
	}
	if event.EventType != "pending" {
		t.Fatalf("EventType = %q, want pending", event.EventType)
	}
}

func TestVerifyWebhookRejectsMissingFields(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"order_id": "order-123",
		// status_code intentionally omitted
		"gross_amount":  "10000",
		"signature_key": "abc",
	})

	adapter := New(WithServerKey("server_key"), WithBaseURL("https://example.com"))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	}); err == nil {
		t.Fatal("VerifyWebhook returned nil error for missing fields")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func response(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
