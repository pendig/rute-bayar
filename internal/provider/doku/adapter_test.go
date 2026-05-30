package doku

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestCreatePaymentUsesCheckoutEndpoint(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != checkoutPaymentPath {
			t.Fatalf("path = %q, want %s", r.URL.Path, checkoutPaymentPath)
		}
		if r.Header.Get("Client-Id") != "client-id" {
			t.Fatalf("Client-Id = %q, want client-id", r.Header.Get("Client-Id"))
		}
		if !strings.HasPrefix(r.Header.Get("Signature"), "HMACSHA256=") {
			t.Fatalf("Signature = %q, want HMACSHA256 prefix", r.Header.Get("Signature"))
		}
		if r.Header.Get("Digest") == "" {
			t.Fatal("Digest header is empty")
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode request returned error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message":["SUCCESS"],
			"response":{
				"order":{"amount":"20000","invoice_number":"rb-doku-001","currency":"IDR","session_id":"session-123"},
				"payment":{"payment_method_types":["VIRTUAL_ACCOUNT_BCA"],"url":"https://sandbox.doku.com/checkout-link-v2/token-123","expired_date":"20240712104711"},
				"headers":{"request_id":"request-123","client_id":"client-id"}
			}
		}`))
	}))
	defer server.Close()

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"), WithBaseURL(server.URL))
	result, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef:     "rb-doku-001",
		Amount:          20000,
		Currency:        "IDR",
		Method:          "bank_transfer",
		Channel:         "bca",
		NotificationURL: "https://example.com/webhooks/doku",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if result.ProviderReference != "rb-doku-001" {
		t.Fatalf("ProviderReference = %q, want rb-doku-001", result.ProviderReference)
	}
	if result.PaymentSessionID != "session-123" {
		t.Fatalf("PaymentSessionID = %q, want session-123", result.PaymentSessionID)
	}
	if result.RedirectURL == "" {
		t.Fatal("RedirectURL is empty")
	}
	if result.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want pending", result.Status)
	}
	payment := captured["payment"].(map[string]any)
	methods := payment["payment_method_types"].([]any)
	if methods[0] != "VIRTUAL_ACCOUNT_BCA" {
		t.Fatalf("payment_method_types[0] = %v, want VIRTUAL_ACCOUNT_BCA", methods[0])
	}
	additionalInfo := captured["additional_info"].(map[string]any)
	if additionalInfo["override_notification_url"] != "https://example.com/webhooks/doku" {
		t.Fatalf("override_notification_url = %v", additionalInfo["override_notification_url"])
	}
}

func TestCreatePaymentSetsDokuWalletNotifyURLForEwallet(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode request returned error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message":["SUCCESS"],
			"response":{
				"order":{"amount":"15000","invoice_number":"rb-doku-ovo-001","currency":"IDR","session_id":"session-123"},
				"payment":{"payment_method_types":["EMONEY_OVO"],"url":"https://sandbox.doku.com/checkout-link-v2/token-123","expired_date":"20240712104711"},
				"headers":{"request_id":"request-123","client_id":"client-id"}
			}
		}`))
	}))
	defer server.Close()

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"), WithBaseURL(server.URL))
	if _, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		ExternalRef:     "rb-doku-ovo-001",
		Amount:          15000,
		Currency:        "IDR",
		Method:          "ewallet",
		Channel:         "ovo",
		NotificationURL: "https://example.com/webhooks/doku",
	}); err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	additionalInfo := captured["additional_info"].(map[string]any)
	if additionalInfo["override_notification_url"] != "https://example.com/webhooks/doku" {
		t.Fatalf("override_notification_url = %v, want https://example.com/webhooks/doku", additionalInfo["override_notification_url"])
	}
	if additionalInfo["doku_wallet_notify_url"] != "https://example.com/webhooks/doku" {
		t.Fatalf("doku_wallet_notify_url = %v, want https://example.com/webhooks/doku", additionalInfo["doku_wallet_notify_url"])
	}
}

func TestGetPaymentStatusMapsDokuStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders/v1/status/rb-doku-001" {
			t.Fatalf("path = %q, want /orders/v1/status/rb-doku-001", r.URL.Path)
		}
		if r.Header.Get("Digest") != "" {
			t.Fatalf("Digest = %q, want empty for GET", r.Header.Get("Digest"))
		}
		_, _ = w.Write([]byte(`{
			"order":{"invoice_number":"rb-doku-001","amount":20000,"status":"ORDER_GENERATED"},
			"transaction":{"status":"SUCCESS","original_request_id":"request-123"},
			"service":{"id":"VIRTUAL_ACCOUNT"},
			"channel":{"id":"VIRTUAL_ACCOUNT_BCA"},
			"virtual_account_info":{"virtual_account_number":"1234567890"}
		}`))
	}))
	defer server.Close()

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"), WithBaseURL(server.URL))
	result, err := adapter.GetPaymentStatus(context.Background(), "rb-doku-001")
	if err != nil {
		t.Fatalf("GetPaymentStatus returned error: %v", err)
	}
	if result.Status != domain.PaymentStatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.VANumber != "1234567890" {
		t.Fatalf("VANumber = %q, want 1234567890", result.VANumber)
	}
	if result.PaymentType != "VIRTUAL_ACCOUNT_BCA" {
		t.Fatalf("PaymentType = %q, want VIRTUAL_ACCOUNT_BCA", result.PaymentType)
	}
}

func TestAuthFailsOnInvalidSignature(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_signature","message":"Invalid Header Signature"}}`))
	}))
	defer server.Close()

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"), WithBaseURL(server.URL))
	_, err := adapter.TestAuth(context.Background())
	if err == nil {
		t.Fatal("TestAuth returned nil error, want invalid signature error")
	}
	if !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("TestAuth error = %q, want invalid signature", err.Error())
	}
}

func TestVerifyAndParseWebhook(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"order":{"invoice_number":"rb-doku-001","amount":20000},
		"transaction":{"status":"SUCCESS","original_request_id":"request-123"},
		"channel":{"id":"VIRTUAL_ACCOUNT_BCA"}
	}`)
	requestID := "webhook-request-123"
	timestamp := "2026-05-30T00:00:00Z"
	signature := dokuSignature("client-id", requestID, timestamp, "/webhooks/doku", dokuDigest(body), "secret")

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"))
	req := provider.WebhookRequest{
		Headers: http.Header{
			"Client-Id":         []string{"client-id"},
			"Request-Id":        []string{requestID},
			"Request-Timestamp": []string{timestamp},
			"Signature":         []string{signature},
		},
		Body:       body,
		TargetPath: "/webhooks/doku",
	}

	if err := adapter.VerifyWebhook(context.Background(), req); err != nil {
		t.Fatalf("VerifyWebhook returned error: %v", err)
	}
	event, err := adapter.ParseWebhook(context.Background(), req)
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.PaymentRef != "rb-doku-001" {
		t.Fatalf("PaymentRef = %q, want rb-doku-001", event.PaymentRef)
	}
	if event.Status != domain.PaymentStatusPaid {
		t.Fatalf("Status = %q, want paid", event.Status)
	}
	if event.ProviderEventID == "" {
		t.Fatal("ProviderEventID is empty")
	}
}

func TestParseWebhookTreatsCheckoutFailedAsPending(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"order":{"invoice_number":"rb-doku-002","amount":20000},
		"transaction":{"status":"FAILED","original_request_id":"request-456"},
		"channel":{"id":"VIRTUAL_ACCOUNT_BCA"}
	}`)
	requestID := "webhook-request-456"
	timestamp := "2026-05-30T00:00:00Z"
	signature := dokuSignature("client-id", requestID, timestamp, "/webhooks/doku", dokuDigest(body), "secret")

	adapter := New(WithClientID("client-id"), WithSecretKey("secret"))
	req := provider.WebhookRequest{
		Headers: http.Header{
			"Client-Id":         []string{"client-id"},
			"Request-Id":        []string{requestID},
			"Request-Timestamp": []string{timestamp},
			"Signature":         []string{signature},
		},
		Body:       body,
		TargetPath: "/webhooks/doku",
	}

	event, err := adapter.ParseWebhook(context.Background(), req)
	if err != nil {
		t.Fatalf("ParseWebhook returned error: %v", err)
	}
	if event.PaymentRef != "rb-doku-002" {
		t.Fatalf("PaymentRef = %q, want rb-doku-002", event.PaymentRef)
	}
	if event.Status != domain.PaymentStatusPending {
		t.Fatalf("Status = %q, want pending", event.Status)
	}
}

func TestDokuPaymentMethodTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		channel string
		want    string
	}{
		{name: "checkout all", method: "checkout", want: ""},
		{name: "bank", method: "bank_transfer", channel: "mandiri", want: "VIRTUAL_ACCOUNT_BANK_MANDIRI"},
		{name: "qris", method: "qris", want: "QRIS"},
		{name: "ewallet", method: "ewallet", channel: "dana", want: "EMONEY_DANA"},
		{name: "raw", method: "VIRTUAL_ACCOUNT_BNI", want: "VIRTUAL_ACCOUNT_BNI"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dokuPaymentMethodTypes(tt.method, tt.channel)
			if err != nil {
				t.Fatalf("dokuPaymentMethodTypes returned error: %v", err)
			}
			if tt.want == "" && len(got) != 0 {
				t.Fatalf("got %v, want empty", got)
			}
			if tt.want != "" && (len(got) != 1 || got[0] != tt.want) {
				t.Fatalf("got %v, want [%s]", got, tt.want)
			}
		})
	}
}
