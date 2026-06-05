package ipaymu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

func TestGenerateSignature(t *testing.T) {
	payload := map[string]any{"account": "1179000899", "transactionId": "4719"}
	got := GenerateSignature("POST", "1179000899", "secret-key", payload)
	want := "27af0b21d04496831361c37237fce28edcfab09e3914d0a9cc3e069c46b862e3"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestGenerateSignatureNilPayloadHashesEmptyBody(t *testing.T) {
	got := GenerateSignature("GET", "1179000899", "secret-key", nil)
	want := "ef60100ee3e1416d08e88f667597c1eff1d95bcd2de54339db504d53d0900558"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestAuthSignsEmptyGETBodyAndCompactTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/payment-channels" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "" {
			t.Fatalf("GET body = %q, want empty", string(body))
		}
		if got, want := r.Header.Get("signature"), GenerateSignature("GET", "1179000899", "secret-key", nil); got != want {
			t.Fatalf("signature = %s, want %s", got, want)
		}
		if got, want := r.Header.Get("timestamp"), "20260604200000"; got != want {
			t.Fatalf("timestamp = %s, want %s", got, want)
		}
		_, _ = w.Write([]byte(`{"Status":200,"Message":"ok"}`))
	}))
	defer server.Close()
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"), WithBaseURL(server.URL), WithTimestamp(func() time.Time { return time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC) }))
	if _, err := adapter.TestAuth(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAuthRejectsProviderAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Status":401,"Message":"unauthorized"}`))
	}))
	defer server.Close()
	adapter := New(WithVA("1179000899"), WithAPIKey("bad-secret"), WithBaseURL(server.URL))
	_, err := adapter.TestAuth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %v, want status 401", err)
	}
}

func TestCreateRedirectPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/payment" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("va") != "1179000899" || r.Header.Get("signature") == "" {
			t.Fatalf("missing auth headers")
		}
		var payload struct {
			ReferenceID string `json:"referenceId"`
			Account     string `json:"account"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ReferenceID != "INV-1" || payload.Account != "1179000899" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"Status":200,"Data":{"SessionID":"SID-1","Url":"https://sandbox.ipaymu.com/payment/SID-1"},"Message":"success"}`))
	}))
	defer server.Close()
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"), WithBaseURL(server.URL), WithTimestamp(func() time.Time { return time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC) }))
	res, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{ExternalRef: "INV-1", Amount: 51000, Method: "redirect", NotificationURL: "https://example.test/webhooks/ipaymu"})
	if err != nil {
		t.Fatal(err)
	}
	if res.PaymentSessionID != "SID-1" || !strings.Contains(res.RedirectURL, "SID-1") {
		t.Fatalf("unexpected response: %#v", res)
	}
}

func TestCreatePaymentValidatesNotificationURL(t *testing.T) {
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"))
	_, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{ExternalRef: "INV-1", Amount: 10000, Method: "redirect"})
	if err == nil || !strings.Contains(err.Error(), "notification url") {
		t.Fatalf("error = %v, want notification url", err)
	}
}

func TestCreateDirectPaymentValidatesRequiredCustomerFields(t *testing.T) {
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"))
	_, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{ExternalRef: "INV-1", Amount: 10000, Method: "qris", NotificationURL: "https://example.test/webhooks/ipaymu", CustomerEmail: "buyer@example.test"})
	if err == nil || !strings.Contains(err.Error(), "customer phone") {
		t.Fatalf("error = %v, want customer phone", err)
	}
	_, err = adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{ExternalRef: "INV-1", Amount: 10000, Method: "qris", NotificationURL: "https://example.test/webhooks/ipaymu", CustomerPhone: "08123456789"})
	if err == nil || !strings.Contains(err.Error(), "customer email") {
		t.Fatalf("error = %v, want customer email", err)
	}
}

func TestCreateDirectPaymentAcceptsStringTotals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/payment/direct" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"Status":200,"Data":{"SessionId":"SID-2","TransactionId":210898,"ReferenceId":"INV-2","Via":"qris","PaymentNo":"PAY-1","Total":"500000","Fee":"0","Expired":"2026-06-06T00:00:00Z"},"Message":"success"}`))
	}))
	defer server.Close()
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"), WithBaseURL(server.URL))
	res, err := adapter.CreatePayment(context.Background(), provider.CreatePaymentRequest{ExternalRef: "INV-2", Amount: 500000, Method: "qris", NotificationURL: "https://example.test/webhooks/ipaymu", CustomerPhone: "08123456789", CustomerEmail: "buyer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TransactionID != "210898" || res.OrderID != "INV-2" {
		t.Fatalf("unexpected response: %#v", res)
	}
}

func TestParseWebhook(t *testing.T) {
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"))
	event, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{Headers: http.Header{"X-Test": {"1"}}, Body: []byte("trx_id=184854&sid=f5aaa61d&reference_id=ID1234&status=berhasil&status_code=1")})
	if err != nil {
		t.Fatal(err)
	}
	if event.PaymentRef != "ID1234" || event.Status != domain.PaymentStatusPaid {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestParseWebhookRejectsMalformedPayload(t *testing.T) {
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"))
	if _, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{Body: []byte("trx_id=184854&reference_id=ID1234")}); err == nil {
		t.Fatal("ParseWebhook returned nil error for missing status_code")
	}
	if _, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{Body: []byte("status_code=bad&trx_id=184854")}); err == nil {
		t.Fatal("ParseWebhook returned nil error for invalid status_code")
	}
	if _, err := adapter.ParseWebhook(context.Background(), provider.WebhookRequest{Body: []byte("status_code=1")}); err == nil {
		t.Fatal("ParseWebhook returned nil error for missing identifiers")
	}
}

func TestVerifyWebhookRequiresSignature(t *testing.T) {
	adapter := New(WithVA("1179000899"), WithAPIKey("secret-key"))
	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{Body: []byte("status_code=1")}); err == nil {
		t.Fatal("VerifyWebhook returned nil error for missing signature")
	}
}

func TestMapStatusCode(t *testing.T) {
	cases := map[int]domain.PaymentStatus{-2: domain.PaymentStatusExpired, 0: domain.PaymentStatusPending, 1: domain.PaymentStatusPaid, 2: domain.PaymentStatusFailed, 3: domain.PaymentStatusRefunded, 4: domain.PaymentStatusFailed, 5: domain.PaymentStatusFailed, 6: domain.PaymentStatusPaid, 7: domain.PaymentStatusPaid}
	for input, want := range cases {
		if got := MapStatusCode(input); got != want {
			t.Fatalf("%d got %s want %s", input, got, want)
		}
	}
}
