package ipaymu

import (
	"context"
	"encoding/json"
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

func TestMapStatusCode(t *testing.T) {
	cases := map[int]domain.PaymentStatus{-2: domain.PaymentStatusExpired, 0: domain.PaymentStatusPending, 1: domain.PaymentStatusPaid, 2: domain.PaymentStatusCancelled, 3: domain.PaymentStatusRefunded, 4: domain.PaymentStatusFailed, 5: domain.PaymentStatusFailed, 6: domain.PaymentStatusPaid, 7: domain.PaymentStatusPaid}
	for input, want := range cases {
		if got := MapStatusCode(input); got != want {
			t.Fatalf("%d got %s want %s", input, got, want)
		}
	}
}
