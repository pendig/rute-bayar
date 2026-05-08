package paymentsvc

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestStatusDoesNotDowngradeRefundedIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox",
		CredentialJSON: []byte(`{"secret_key":"secret"}`),
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-xnd-refunded",
		ProviderCode: domain.ProviderXendit,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusRefunded,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}
	_, err = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderXendit,
		RequestJSON:       []byte(`{"request":"create"}`),
		ResponseJSON:      []byte(`{"response":"create"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "ps_refunded",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}

	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"payment_session_id":"ps_refunded",
			"reference_id":"rb-xnd-refunded",
			"status":"COMPLETED",
			"mode":"PAYMENT_LINK"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Status(ctx, StatusInput{
		Provider:    domain.ProviderXendit,
		Environment: domain.EnvironmentSandbox,
		BaseURL:     "https://example.com",
		Reference:   "rb-xnd-refunded",
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if result.Response.Status != domain.PaymentStatusSettled {
		t.Fatalf("provider status = %q, want settled", result.Response.Status)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-refunded")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusRefunded {
		t.Fatalf("intent status = %q, want refunded", intent.Status)
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

func TestCreateReturnsProviderResponseWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := &testPaymentStore{
		account: domain.ProviderAccount{
			ProviderCode:   domain.ProviderXendit,
			Environment:    domain.EnvironmentSandbox,
			DisplayName:    "Xendit Sandbox",
			CredentialJSON: []byte(`{"secret_key":"secret"}`),
			ConfigJSON:     []byte(`{}`),
		},
		recordAttemptErr: errors.New("persist failed"),
	}

	var requestPath string
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		return response(http.StatusOK, `{
			"id":"ps-xnd-002",
			"reference_id":"rb-xnd-002",
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
		ExternalRef:  "rb-xnd-002",
		Amount:       15000,
		Currency:     "IDR",
		Method:       "bank_transfer",
		CustomerName: "Rute Bayar",
	})
	if err == nil {
		t.Fatal("Create returned nil error when persistence failed")
	}
	if result.ProviderCode != domain.ProviderXendit {
		t.Fatalf("ProviderCode = %q, want xendit", result.ProviderCode)
	}
	if result.Reference != "rb-xnd-002" {
		t.Fatalf("Reference = %q, want rb-xnd-002", result.Reference)
	}
	if result.Response.RedirectURL != "https://example.com/pay" {
		t.Fatalf("RedirectURL = %q, want payment link", result.Response.RedirectURL)
	}
	if requestPath != "/sessions" {
		t.Fatalf("request path = %q, want /sessions", requestPath)
	}
	if len(store.attempts) != 1 {
		t.Fatalf("attempts recorded = %d, want 1", len(store.attempts))
	}
}

func TestStatusReturnsProviderStatusWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := &testPaymentStore{
		account: domain.ProviderAccount{
			ProviderCode:   domain.ProviderMidtrans,
			Environment:    domain.EnvironmentSandbox,
			DisplayName:    "Midtrans Sandbox",
			CredentialJSON: []byte(`{"merchant_id":"merchant","client_key":"client","server_key":"server"}`),
			ConfigJSON:     []byte(`{}`),
		},
		intents: map[string]domain.PaymentIntent{
			"rb-mid-002": {
				ID:           "intent-002",
				ExternalRef:  "rb-mid-002",
				ProviderCode: domain.ProviderMidtrans,
				Amount:       15000,
				Currency:     "IDR",
				Status:       domain.PaymentStatusPending,
			},
		},
		upsertErr: errors.New("persist failed"),
	}

	var requestPath string
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		return response(http.StatusOK, `{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-mid-002",
			"order_id":"order-mid-002",
			"transaction_status":"settlement",
			"fraud_status":"accept",
			"payment_type":"bank_transfer"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Status(ctx, StatusInput{
		Provider:          domain.ProviderMidtrans,
		Environment:       domain.EnvironmentSandbox,
		BaseURL:           "https://example.com",
		Reference:         "rb-mid-002",
		ProviderReference: "order-mid-002",
	})
	if err == nil {
		t.Fatal("Status returned nil error when persistence failed")
	}
	if result.ProviderCode != domain.ProviderMidtrans {
		t.Fatalf("ProviderCode = %q, want midtrans", result.ProviderCode)
	}
	if result.Reference != "rb-mid-002" {
		t.Fatalf("Reference = %q, want rb-mid-002", result.Reference)
	}
	if result.Response.Status != domain.PaymentStatusSettled {
		t.Fatalf("Status = %q, want settled", result.Response.Status)
	}
	if requestPath != "/v2/order-mid-002/status" {
		t.Fatalf("request path = %q, want /v2/order-mid-002/status", requestPath)
	}
	if len(store.statusChecks) != 0 {
		t.Fatalf("status checks recorded = %d, want 0 when upsert fails first", len(store.statusChecks))
	}
}

func TestRefundXenditRecordsRefundAndUpdatesIntent(t *testing.T) {
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

	var requestBody map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/sessions/ps-xnd-001":
			return response(http.StatusOK, `{
				"payment_session_id":"ps-xnd-001",
				"reference_id":"rb-xnd-refund",
				"payment_request_id":"pr-xnd-001",
				"status":"COMPLETED",
				"payment_link_url":"https://example.com/pay"
			}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/refunds":
			if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
				t.Fatalf("decode refund request: %v", err)
			}
			if _, ok := requestBody["amount"]; ok {
				t.Fatalf("refund request should omit amount for full refund, got %v", requestBody["amount"])
			}
			return response(http.StatusOK, `{
				"id":"rfd-001",
				"payment_request_id":"pr-xnd-001",
				"status":"SUCCEEDED",
				"reason":"REQUESTED_BY_CUSTOMER"
			}`), nil
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Refund(ctx, RefundInput{
		Provider:          domain.ProviderXendit,
		Environment:       domain.EnvironmentSandbox,
		BaseURL:           "https://example.com",
		Reference:         "rb-xnd-refund",
		ProviderReference: "ps-xnd-001",
		RefundReference:   "rb-xnd-refund-001",
		Amount:            0,
		Currency:          "IDR",
		Reason:            "requested by customer",
	})
	if err != nil {
		t.Fatalf("Refund returned error: %v", err)
	}
	if result.ProviderReference != "ps-xnd-001" {
		t.Fatalf("ProviderReference = %q, want ps-xnd-001", result.ProviderReference)
	}
	if result.Response.Status != domain.PaymentStatusRefunded {
		t.Fatalf("Response.Status = %q, want refunded", result.Response.Status)
	}
	if requestBody["reference_id"] != "rb-xnd-refund-001" {
		t.Fatalf("request reference_id = %v, want refund reference", requestBody["reference_id"])
	}
	if requestBody["payment_request_id"] != "pr-xnd-001" {
		t.Fatalf("request payment_request_id = %v, want pr-xnd-001", requestBody["payment_request_id"])
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-refund")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusRefunded {
		t.Fatalf("intent status = %q, want refunded", intent.Status)
	}
	refund, err := store.GetLatestRefundByIntent(ctx, intentID, domain.ProviderXendit)
	if err != nil {
		t.Fatalf("GetLatestRefundByIntent returned error: %v", err)
	}
	if refund.Status != domain.PaymentStatusRefunded {
		t.Fatalf("refund status = %q, want refunded", refund.Status)
	}
	if refund.Amount != 15000 {
		t.Fatalf("refund amount = %d, want intent amount 15000", refund.Amount)
	}
}

func TestRefundReturnsProviderErrorAndRecordsFailedRefund(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := &testPaymentStore{
		account: domain.ProviderAccount{
			ProviderCode:   domain.ProviderMidtrans,
			Environment:    domain.EnvironmentSandbox,
			DisplayName:    "Midtrans Sandbox",
			CredentialJSON: []byte(`{"merchant_id":"merchant","client_key":"client","server_key":"server"}`),
			ConfigJSON:     []byte(`{}`),
		},
		intents: map[string]domain.PaymentIntent{
			"rb-mid-refund": {
				ID:           "intent-mid-refund",
				ExternalRef:  "rb-mid-refund",
				ProviderCode: domain.ProviderMidtrans,
				Amount:       15000,
				Currency:     "IDR",
				Status:       domain.PaymentStatusSettled,
			},
		},
	}

	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusBadRequest, `{"status_code":"400","status_message":"Invalid refund"}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Refund(ctx, RefundInput{
		Provider:          domain.ProviderMidtrans,
		Environment:       domain.EnvironmentSandbox,
		BaseURL:           "https://example.com",
		Reference:         "rb-mid-refund",
		ProviderReference: "order-mid-refund",
		RefundReference:   "order-mid-refund-r1",
		Amount:            5000,
		Currency:          "IDR",
		Reason:            "requested by customer",
	})
	if err == nil {
		t.Fatal("Refund returned nil error for provider failure")
	}
	if result.Response.Status != domain.PaymentStatusFailed {
		t.Fatalf("Response.Status = %q, want failed", result.Response.Status)
	}
	if len(store.refunds) != 1 {
		t.Fatalf("refunds recorded = %d, want 1", len(store.refunds))
	}
	if len(store.intents) == 0 {
		t.Fatal("intent map is empty")
	}
	if store.intents["rb-mid-refund"].Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want unchanged settled", store.intents["rb-mid-refund"].Status)
	}
}

func TestReconcileUpdatesMismatchedIntentStatus(t *testing.T) {
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

	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"status_code":"200",
			"status_message":"Success, transaction is found",
			"transaction_id":"tx-mid-reconcile",
			"order_id":"order-mid-reconcile",
			"transaction_status":"settlement",
			"fraud_status":"accept",
			"payment_type":"bank_transfer"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Reconcile(ctx, ReconcileInput{
		Provider:    domain.ProviderMidtrans,
		Environment: domain.EnvironmentSandbox,
		BaseURL:     "https://example.com",
		Reference:   "rb-mid-reconcile",
	})
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.Matched {
		t.Fatal("Matched = true, want false before update")
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true when local status differed")
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-mid-reconcile")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusSettled {
		t.Fatalf("intent status = %q, want settled", intent.Status)
	}
}

func TestReconcileDoesNotDowngradeRefundedIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox",
		CredentialJSON: []byte(`{"secret_key":"secret"}`),
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  "rb-xnd-reconcile-refunded",
		ProviderCode: domain.ProviderXendit,
		Amount:       15000,
		Currency:     "IDR",
		Status:       domain.PaymentStatusRefunded,
	})
	if err != nil {
		t.Fatalf("UpsertPaymentIntent returned error: %v", err)
	}
	_, err = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderXendit,
		RequestJSON:       []byte(`{"request":"create"}`),
		ResponseJSON:      []byte(`{"response":"create"}`),
		Status:            domain.PaymentStatusPending,
		ProviderReference: "ps_reconcile_refunded",
	})
	if err != nil {
		t.Fatalf("RecordPaymentAttempt returned error: %v", err)
	}

	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"payment_session_id":"ps_reconcile_refunded",
			"reference_id":"rb-xnd-reconcile-refunded",
			"status":"COMPLETED",
			"mode":"PAYMENT_LINK"
		}`), nil
	})}

	svc := New(store, providerfactory.New(store, providerfactory.WithHTTPClient(client)))
	result, err := svc.Reconcile(ctx, ReconcileInput{
		Provider:    domain.ProviderXendit,
		Environment: domain.EnvironmentSandbox,
		BaseURL:     "https://example.com",
		Reference:   "rb-xnd-reconcile-refunded",
	})
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.Matched {
		t.Fatal("Matched = true, want false because provider session still reports settled")
	}
	if result.Updated {
		t.Fatal("Updated = true, want false for refunded local status")
	}
	if result.ProviderStatus != domain.PaymentStatusSettled {
		t.Fatalf("ProviderStatus = %q, want settled", result.ProviderStatus)
	}

	intent, err := store.GetPaymentIntentByExternalRef(ctx, "rb-xnd-reconcile-refunded")
	if err != nil {
		t.Fatalf("GetPaymentIntentByExternalRef returned error: %v", err)
	}
	if intent.Status != domain.PaymentStatusRefunded {
		t.Fatalf("intent status = %q, want refunded", intent.Status)
	}
}

func TestReconcileRejectsMissingIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	svc := New(store, providerfactory.New(store))
	if _, err := svc.Reconcile(ctx, ReconcileInput{
		Provider:    domain.ProviderXendit,
		Environment: domain.EnvironmentSandbox,
		Reference:   "missing-ref",
	}); err == nil {
		t.Fatal("Reconcile returned nil error for missing intent")
	}
}

type testPaymentStore struct {
	account          domain.ProviderAccount
	intents          map[string]domain.PaymentIntent
	attempts         []domain.PaymentAttempt
	statusChecks     []domain.PaymentStatusCheck
	refunds          []domain.Refund
	upsertErr        error
	recordAttemptErr error
	statusCheckErr   error
	recordRefundErr  error
}

func (s *testPaymentStore) GetProviderAccount(_ context.Context, provider domain.ProviderCode, environment domain.Environment) (domain.ProviderAccount, error) {
	if s.account.ProviderCode != provider || s.account.Environment != environment {
		return domain.ProviderAccount{}, sql.ErrNoRows
	}
	return s.account, nil
}

func (s *testPaymentStore) UpsertPaymentIntent(_ context.Context, intent domain.PaymentIntent) (string, error) {
	if s.upsertErr != nil {
		return "", s.upsertErr
	}
	if s.intents == nil {
		s.intents = map[string]domain.PaymentIntent{}
	}
	if intent.ID == "" {
		intent.ID = "intent-generated"
	}
	s.intents[strings.TrimSpace(intent.ExternalRef)] = intent
	return intent.ID, nil
}

func (s *testPaymentStore) RecordPaymentAttempt(_ context.Context, attempt domain.PaymentAttempt) (string, error) {
	s.attempts = append(s.attempts, attempt)
	if s.recordAttemptErr != nil {
		return "", s.recordAttemptErr
	}
	return "attempt-1", nil
}

func (s *testPaymentStore) RecordPaymentStatusCheck(_ context.Context, check domain.PaymentStatusCheck) (string, error) {
	s.statusChecks = append(s.statusChecks, check)
	if s.statusCheckErr != nil {
		return "", s.statusCheckErr
	}
	return "status-check-1", nil
}

func (s *testPaymentStore) RecordRefund(_ context.Context, refund domain.Refund) (string, error) {
	s.refunds = append(s.refunds, refund)
	if s.recordRefundErr != nil {
		return "", s.recordRefundErr
	}
	return "refund-1", nil
}

func (s *testPaymentStore) GetPaymentIntentByExternalRef(_ context.Context, externalRef string) (domain.PaymentIntent, error) {
	if s.intents == nil {
		return domain.PaymentIntent{}, sql.ErrNoRows
	}
	intent, ok := s.intents[strings.TrimSpace(externalRef)]
	if !ok {
		return domain.PaymentIntent{}, sql.ErrNoRows
	}
	return intent, nil
}

func (s *testPaymentStore) GetLatestPaymentAttemptByIntent(_ context.Context, paymentIntentID string, provider domain.ProviderCode) (domain.PaymentAttempt, error) {
	for i := len(s.attempts) - 1; i >= 0; i-- {
		attempt := s.attempts[i]
		if attempt.PaymentIntentID == paymentIntentID && attempt.ProviderCode == provider {
			return attempt, nil
		}
	}
	return domain.PaymentAttempt{}, sql.ErrNoRows
}
