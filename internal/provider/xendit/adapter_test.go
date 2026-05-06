package xendit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestTestAuthSendsBasicAuth(t *testing.T) {
	t.Parallel()

	var authHeader string
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/balance" {
			t.Fatalf("request path = %q, want /balance", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		return response(http.StatusOK, `{"balance":1000}`), nil
	})}

	adapter := New(WithSecretKey("secret_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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

	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusUnauthorized, `{"error_code":"INVALID_API_KEY"}`), nil
	})}

	adapter := New(WithSecretKey("bad_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for unauthorized response")
	}
}

func TestTestAuthAllowsForbiddenBalancePermission(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusForbidden, `{"error_code":"REQUEST_FORBIDDEN_ERROR"}`), nil
	})}

	adapter := New(WithSecretKey("money_in_only_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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

	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `not-json`), nil
	})}

	adapter := New(WithSecretKey("secret_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	if _, err := adapter.TestAuth(context.Background()); err == nil {
		t.Fatal("TestAuth returned nil error for malformed JSON")
	}
}

func TestGetPaymentStatusMapsActiveSession(t *testing.T) {
	t.Parallel()

	var requestedPath string
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requestedPath = r.URL.Path
		return response(http.StatusOK, `{
			"id":"ps_123",
			"reference_id":"rb-001",
			"mode":"PAYMENT_LINK",
			"status":"ACTIVE",
			"payment_link_url":"https://example.com/pay"
		}`), nil
	})}

	adapter := New(WithSecretKey("secret_key"), WithBaseURL("https://example.com"), WithHTTPClient(client))
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

func TestCreatePaymentCreatesSession(t *testing.T) {
	t.Parallel()

	var requestPath string
	var rawBody []byte
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requestPath = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		rawBody = raw
		return response(http.StatusCreated, `{
			"id":"sess_01",
			"reference_id":"rb-001",
			"mode":"PAYMENT_LINK",
			"status":"ACTIVE",
			"payment_link_url":"https://checkout.example.com/pay"
		}`), nil
	})}

	adapter := New(WithSecretKey("secret"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef: "rb-001",
		Amount:      15000,
		Currency:    "IDR",
		Method:      "payment_link",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if requestPath != "/sessions" {
		t.Fatalf("requested path = %q, want /sessions", requestPath)
	}
	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}
	if payload["reference_id"] != "rb-001" {
		t.Fatalf("reference_id = %v, want rb-001", payload["reference_id"])
	}
	if payload["mode"] != "PAYMENT_LINK" {
		t.Fatalf("mode = %v, want PAYMENT_LINK", payload["mode"])
	}
	if result.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want %q", result.Status, domain.PaymentStatusPending)
	}
	if result.TransactionID != "sess_01" {
		t.Fatalf("TransactionID = %q, want sess_01", result.TransactionID)
	}
}

func TestCreatePaymentRequiresPaymentLinkMethod(t *testing.T) {
	t.Parallel()

	adapter := New(WithSecretKey("secret"))
	if _, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef: "rb-001",
		Amount:      1000,
		Method:      "bank_transfer",
	}); err == nil {
		t.Fatal("CreatePayment returned nil error for unsupported method")
	}
}

func TestRefundPaymentResolvesSessionToPaymentRequestID(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sessions/ps_123":
			return response(http.StatusOK, `{
				"payment_session_id":"ps_123",
				"reference_id":"rb-001",
				"payment_request_id":"pr_123",
				"status":"COMPLETED",
				"payment_link_url":"https://example.com/pay"
			}`), nil
		case r.Method == http.MethodPost && r.URL.Path == "/refunds":
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatalf("decode refund request: %v", err)
			}
			return response(http.StatusOK, `{
				"id":"rfd_123",
				"payment_request_id":"pr_123",
				"status":"SUCCEEDED",
				"reason":"REQUESTED_BY_CUSTOMER"
			}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}

	adapter := New(WithSecretKey("secret"), WithBaseURL("https://example.com"), WithHTTPClient(client))
	result, err := adapter.RefundPayment(context.Background(), provider.RefundRequest{
		ProviderReference: "ps_123",
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
	if result.ProviderReference != "ps_123" {
		t.Fatalf("ProviderReference = %q, want ps_123", result.ProviderReference)
	}
	if result.PaymentRequestID != "pr_123" {
		t.Fatalf("PaymentRequestID = %q, want pr_123", result.PaymentRequestID)
	}
	if requestBody["payment_request_id"] != "pr_123" {
		t.Fatalf("payment_request_id = %v, want pr_123", requestBody["payment_request_id"])
	}
	if requestBody["reference_id"] != "refund-001" {
		t.Fatalf("reference_id = %v, want refund-001", requestBody["reference_id"])
	}
	if requestBody["reason"] != "REQUESTED_BY_CUSTOMER" {
		t.Fatalf("reason = %v, want REQUESTED_BY_CUSTOMER", requestBody["reason"])
	}
}

func TestVerifyWebhookRejectsWrongToken(t *testing.T) {
	t.Parallel()

	adapter := New(WithSecretKey("secret"), WithCallbackToken("expected-token"))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/xendit", strings.NewReader(`{"status":"PAID"}`))
	req.Header.Set("X-Callback-Token", "wrong-token")
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: req.Header,
		Body:    []byte(`{"status":"PAID"}`),
	}); err == nil {
		t.Fatal("VerifyWebhook returned nil error for wrong callback token")
	}
}

func TestVerifyWebhookRejectsMissingToken(t *testing.T) {
	t.Parallel()

	adapter := New(WithSecretKey("secret"), WithCallbackToken("expected-token"))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/xendit", strings.NewReader(`{"status":"PAID"}`))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: req.Header,
		Body:    []byte(`{"status":"PAID"}`),
	}); err == nil {
		t.Fatal("VerifyWebhook returned nil error for missing callback token")
	}
}

func TestVerifyWebhookAcceptsMatchingToken(t *testing.T) {
	t.Parallel()

	adapter := New(WithSecretKey("secret"), WithCallbackToken("expected-token"))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/xendit", strings.NewReader(`{"status":"PAID"}`))
	req.Header.Set("X-Callback-Token", "expected-token")
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: req.Header,
		Body:    []byte(`{"status":"PAID"}`),
	}); err != nil {
		t.Fatalf("VerifyWebhook returned error for matching token: %v", err)
	}
}

func TestParseWebhookMapsStatus(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"id":           "evt_123",
		"status":       "COMPLETED",
		"event":        "payment.completed",
		"reference_id": "rb-001",
	})

	adapter := New(WithSecretKey("secret"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "payment.completed:evt_123" {
		t.Fatalf("ProviderEventID = %q, want payment.completed:evt_123", event.ProviderEventID)
	}
	if event.EventType != "payment.completed" {
		t.Fatalf("EventType = %q, want payment.completed", event.EventType)
	}
	if event.Status != domain.PaymentStatusSettled {
		t.Fatalf("Status = %q, want %q", event.Status, domain.PaymentStatusSettled)
	}
}

func TestParseWebhookSupportsNestedEnvelope(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"paymentCapture": {
			"value": {
				"event": "payment.capture",
				"created": "2026-05-06T09:00:00.000Z",
				"data": {
					"payment_id": "py_123",
					"reference_id": "rb-001",
					"status": "SUCCEEDED"
				}
			}
		}
	}`)

	adapter := New(WithSecretKey("secret"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "payment.capture:py_123" {
		t.Fatalf("ProviderEventID = %q, want payment.capture:py_123", event.ProviderEventID)
	}
	if event.PaymentRef != "rb-001" {
		t.Fatalf("PaymentRef = %q, want rb-001", event.PaymentRef)
	}
	if event.EventType != "payment.capture" {
		t.Fatalf("EventType = %q, want payment.capture", event.EventType)
	}
	if event.Status != domain.PaymentStatusPaid {
		t.Fatalf("Status = %q, want %q", event.Status, domain.PaymentStatusPaid)
	}
}

func TestParseWebhookFallsBackToStatusEvent(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"id":       "evt_456",
		"status":   "FAILED",
		"order_id": "rb-001",
	})
	adapter := New(WithSecretKey("secret"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.EventType != "FAILED" {
		t.Fatalf("EventType = %q, want FAILED", event.EventType)
	}
	if event.Status != domain.PaymentStatusFailed {
		t.Fatalf("Status = %q, want %q", event.Status, domain.PaymentStatusFailed)
	}
}

func TestParseWebhookUsesReferenceWhenProviderEventIDMissing(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]any{
		"event":        "payment.completed",
		"status":       "SUCCEEDDED",
		"reference_id": "rb-002",
	})
	adapter := New(WithSecretKey("secret"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{
		Headers: nil,
		Body:    payload,
	})
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.ProviderEventID != "payment.completed:rb-002" {
		t.Fatalf("ProviderEventID = %q, want payment.completed:rb-002", event.ProviderEventID)
	}
	if event.PaymentRef != "rb-002" {
		t.Fatalf("PaymentRef = %q, want rb-002", event.PaymentRef)
	}
	if event.Status != domain.PaymentStatusPaid {
		t.Fatalf("Status = %q, want %q", event.Status, domain.PaymentStatusPaid)
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
