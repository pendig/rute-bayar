package providerfactory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
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

func TestMidtransCredentialFromJSON(t *testing.T) {
	t.Parallel()

	credential, err := midtransCredentialFromJSON([]byte(`{
		"merchant_id":" merchant ",
		"client_key":" client ",
		"server_key":" server "
	}`))
	if err != nil {
		t.Fatalf("midtransCredentialFromJSON returned error: %v", err)
	}
	if credential.MerchantID != "merchant" {
		t.Fatalf("MerchantID = %q, want merchant", credential.MerchantID)
	}
	if credential.ClientKey != "client" {
		t.Fatalf("ClientKey = %q, want client", credential.ClientKey)
	}
	if credential.ServerKey != "server" {
		t.Fatalf("ServerKey = %q, want server", credential.ServerKey)
	}
}

func TestMidtransCredentialFromJSONRequiresFields(t *testing.T) {
	t.Parallel()

	if _, err := midtransCredentialFromJSON([]byte(`{"merchant_id":"merchant","client_key":"client"}`)); err == nil {
		t.Fatal("midtransCredentialFromJSON returned nil error for missing server key")
	}
}

func TestXenditSecretKeyFromJSON(t *testing.T) {
	t.Parallel()

	secret, err := xenditSecretKeyFromJSON([]byte(`{"secret_key":" xnd_test_key "}`))
	if err != nil {
		t.Fatalf("xenditSecretKeyFromJSON returned error: %v", err)
	}
	if secret != "xnd_test_key" {
		t.Fatalf("xenditSecretKeyFromJSON = %q, want xnd_test_key", secret)
	}
}

func TestXenditSecretKeyFromJSONRequiresSecret(t *testing.T) {
	t.Parallel()

	if _, err := xenditSecretKeyFromJSON([]byte(`{"secret_key":""}`)); err == nil {
		t.Fatal("xenditSecretKeyFromJSON returned nil error for missing secret")
	}
}

func TestDokuCredentialFromJSON(t *testing.T) {
	t.Parallel()

	credential, err := dokuCredentialFromJSON([]byte(`{"client_id":" client ","secret_key":" secret "}`))
	if err != nil {
		t.Fatalf("dokuCredentialFromJSON returned error: %v", err)
	}
	if credential.ClientID != "client" {
		t.Fatalf("ClientID = %q, want client", credential.ClientID)
	}
	if credential.SecretKey != "secret" {
		t.Fatalf("SecretKey = %q, want secret", credential.SecretKey)
	}
}

func TestDokuCredentialFromJSONRequiresFields(t *testing.T) {
	t.Parallel()

	if _, err := dokuCredentialFromJSON([]byte(`{"client_id":"client"}`)); err == nil {
		t.Fatal("dokuCredentialFromJSON returned nil error for missing secret key")
	}
}

func TestAdapterForAccountBuildsMidtransAdapter(t *testing.T) {
	t.Parallel()

	var (
		requestURL string
		authHeader string
	)
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestURL = req.URL.String()
		authHeader = req.Header.Get("Authorization")
		return response(http.StatusNotFound, `{"status_code":"404","status_message":"Transaction doesn't exist."}`), nil
	})}

	factory := New(nil, WithHTTPClient(client))
	adapter, err := factory.MidtransAdapterForAccount(domain.ProviderAccount{
		ProviderCode: domain.ProviderMidtrans,
		Environment:  domain.EnvironmentSandbox,
		CredentialJSON: mustJSON(t, map[string]string{
			"merchant_id": "merchant",
			"client_key":  "client",
			"server_key":  "server_key",
		}),
	}, "")
	if err != nil {
		t.Fatalf("MidtransAdapterForAccount returned error: %v", err)
	}

	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if info.StatusCode != "404" {
		t.Fatalf("StatusCode = %q, want 404", info.StatusCode)
	}
	if requestURL != "https://api.sandbox.midtrans.com/v2/rute-bayar-auth-test/status" {
		t.Fatalf("request URL = %q, want sandbox auth endpoint", requestURL)
	}
	if authHeader != "Basic "+base64.StdEncoding.EncodeToString([]byte("server_key:")) {
		t.Fatalf("Authorization = %q, want Basic auth for server_key", authHeader)
	}
}

func TestAdapterForAccountBuildsXenditAdapter(t *testing.T) {
	t.Parallel()

	var (
		requestURL string
		authHeader string
	)
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestURL = req.URL.String()
		authHeader = req.Header.Get("Authorization")
		return response(http.StatusOK, `{"balance":1000}`), nil
	})}

	factory := New(nil, WithHTTPClient(client))
	adapter, err := factory.XenditAdapterForAccount(domain.ProviderAccount{
		ProviderCode: domain.ProviderXendit,
		Environment:  domain.EnvironmentSandbox,
		CredentialJSON: mustJSON(t, map[string]string{
			"secret_key": "secret_key",
		}),
		ConfigJSON: mustJSON(t, map[string]string{
			"webhook_token": "expected-token",
		}),
	}, "https://example.com")
	if err != nil {
		t.Fatalf("XenditAdapterForAccount returned error: %v", err)
	}

	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if info.Balance == nil || *info.Balance != 1000 {
		t.Fatalf("Balance = %v, want 1000", info.Balance)
	}
	if requestURL != "https://example.com/balance" {
		t.Fatalf("request URL = %q, want override base URL", requestURL)
	}
	if authHeader != "Basic "+base64.StdEncoding.EncodeToString([]byte("secret_key:")) {
		t.Fatalf("Authorization = %q, want Basic auth for secret_key", authHeader)
	}

	if err := adapter.VerifyWebhook(context.Background(), provider.WebhookRequest{
		Headers: http.Header{"X-Callback-Token": []string{"expected-token"}},
	}); err != nil {
		t.Fatalf("VerifyWebhook returned error with matching token: %v", err)
	}
}

func TestAdapterForAccountBuildsDokuAdapter(t *testing.T) {
	t.Parallel()

	var (
		requestURL string
		clientID   string
		signature  string
	)
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestURL = req.URL.String()
		clientID = req.Header.Get("Client-Id")
		signature = req.Header.Get("Signature")
		return response(http.StatusNotFound, `{"error_messages":["not found"]}`), nil
	})}

	factory := New(nil, WithHTTPClient(client))
	adapter, err := factory.DokuAdapterForAccount(domain.ProviderAccount{
		ProviderCode: domain.ProviderDoku,
		Environment:  domain.EnvironmentSandbox,
		CredentialJSON: mustJSON(t, map[string]string{
			"client_id":  "client-id",
			"secret_key": "secret",
		}),
		ConfigJSON: mustJSON(t, map[string]string{
			"webhook_target_path": "/webhooks/doku",
		}),
	}, "https://example.com")
	if err != nil {
		t.Fatalf("DokuAdapterForAccount returned error: %v", err)
	}

	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if info.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", info.StatusCode)
	}
	if requestURL != "https://example.com/orders/v1/status/rute-bayar-auth-test" {
		t.Fatalf("request URL = %q, want override status endpoint", requestURL)
	}
	if clientID != "client-id" {
		t.Fatalf("Client-Id = %q, want client-id", clientID)
	}
	if !strings.HasPrefix(signature, "HMACSHA256=") {
		t.Fatalf("Signature = %q, want HMACSHA256 prefix", signature)
	}
}

func TestWebhookHandlersAllowUnconfiguredProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	handlers, err := New(store).WebhookHandlers(ctx, domain.EnvironmentSandbox)
	if err != nil {
		t.Fatalf("WebhookHandlers returned error: %v", err)
	}
	if len(handlers) != 0 {
		t.Fatalf("handlers length = %d, want 0", len(handlers))
	}
}

func TestWebhookHandlersBuildsConfiguredProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderMidtrans,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Midtrans Sandbox",
		CredentialJSON: mustJSON(t, map[string]string{"merchant_id": "merchant", "client_key": "client", "server_key": "server"}),
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount midtrans returned error: %v", err)
	}

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Xendit Sandbox",
		CredentialJSON: mustJSON(t, map[string]string{"secret_key": "secret"}),
		ConfigJSON:     mustJSON(t, map[string]string{"webhook_token": "expected-token"}),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount xendit returned error: %v", err)
	}

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderDoku,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Doku Sandbox",
		CredentialJSON: mustJSON(t, map[string]string{"client_id": "client", "secret_key": "secret"}),
		ConfigJSON:     mustJSON(t, map[string]string{"webhook_target_path": "/webhooks/doku"}),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount doku returned error: %v", err)
	}

	handlers, err := New(store).WebhookHandlers(ctx, domain.EnvironmentSandbox)
	if err != nil {
		t.Fatalf("WebhookHandlers returned error: %v", err)
	}
	if len(handlers) != 3 {
		t.Fatalf("handlers length = %d, want 3", len(handlers))
	}
	if err := handlers[domain.ProviderXendit].VerifyWebhook(ctx, provider.WebhookRequest{
		Headers: http.Header{"X-Callback-Token": []string{"expected-token"}},
	}); err != nil {
		t.Fatalf("VerifyWebhook on xendit handler returned error: %v", err)
	}
}

func TestWebhookHandlersReturnsMalformedCredentialError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderMidtrans,
		Environment:    domain.EnvironmentSandbox,
		DisplayName:    "Midtrans Sandbox",
		CredentialJSON: []byte(`{"merchant_id":"merchant","client_key":"client"}`),
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("UpsertProviderAccount returned error: %v", err)
	}

	if _, err := New(store).WebhookHandlers(ctx, domain.EnvironmentSandbox); err == nil {
		t.Fatal("WebhookHandlers returned nil error for malformed credential")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return raw
}
