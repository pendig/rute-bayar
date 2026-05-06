package paymentsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/providerfactory"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCreateXenditCreatesIntentAndAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
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

	var (
		requestPath string
		authHeader  string
		requestBody string
	)
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		authHeader = req.Header.Get("Authorization")
		body, _ := io.ReadAll(req.Body)
		requestBody = string(body)
		return response(http.StatusOK, `{
			"id":"ps-xnd-001",
			"reference_id":"rb-xnd-001",
			"mode":"PAYMENT_LINK",
			"status":"COMPLETED",
			"payment_link_url":"https://example.com/pay"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Create(ctx, CreateInput{
		Provider:     domain.ProviderXendit,
		Environment:  domain.EnvironmentSandbox,
		BaseURL:      "https://example.com",
		ExternalRef:  "rb-xnd-001",
		Amount:       15000,
		Currency:     "IDR",
		Method:       "bank_transfer",
		CustomerName: "Rute Bayar",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.ProviderCode != domain.ProviderXendit {
		t.Fatalf("ProviderCode = %q, want xendit", result.ProviderCode)
	}
	if result.Reference != "rb-xnd-001" {
		t.Fatalf("Reference = %q, want rb-xnd-001", result.Reference)
	}
	if result.Response.Status != domain.PaymentStatusSettled {
		t.Fatalf("Status = %q, want settled", result.Response.Status)
	}
	if requestPath != "/sessions" {
		t.Fatalf("request path = %q, want /sessions", requestPath)
	}
	if authHeader != "Basic "+base64.StdEncoding.EncodeToString([]byte("secret:")) {
		t.Fatalf("Authorization = %q, want Basic auth", authHeader)
	}
	if !strings.Contains(requestBody, `"mode":"PAYMENT_LINK"`) {
		t.Fatalf("request body missing PAYMENT_LINK mode: %s", requestBody)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want settled", intent.Status)
	}
}

func TestStatusMidtransUsesLatestAttemptReference(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
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

	var requestPath string
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		return response(http.StatusOK, `{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-mid-001",
			"order_id":"order-mid-001",
			"transaction_status":"settlement",
			"fraud_status":"accept",
			"payment_type":"bank_transfer"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Status(ctx, StatusInput{
		Provider:    domain.ProviderMidtrans,
		Environment: domain.EnvironmentSandbox,
		BaseURL:     "https://example.com",
		Reference:   "rb-mid-001",
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if result.ProviderReference != "order-mid-001" {
		t.Fatalf("ProviderReference = %q, want order-mid-001", result.ProviderReference)
	}
	if result.Reference != "rb-mid-001" {
		t.Fatalf("Reference = %q, want rb-mid-001", result.Reference)
	}
	if result.Response.Status != domain.PaymentStatusSettled {
		t.Fatalf("Status = %q, want settled", result.Response.Status)
	}
	if requestPath != "/v2/order-mid-001/status" {
		t.Fatalf("request path = %q, want /v2/order-mid-001/status", requestPath)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-mid-001")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want settled", intent.Status)
	}
}

func TestCreateXenditRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if _, err := New(store, nil).Create(ctx, CreateInput{
		Provider:    domain.ProviderXendit,
		Environment: domain.EnvironmentSandbox,
		ExternalRef: "rb-xnd-unsupported",
		Amount:      1000,
		Method:      "virtual_account",
	}); err == nil {
		t.Fatal("Create returned nil error for unsupported xendit method")
	}
}
