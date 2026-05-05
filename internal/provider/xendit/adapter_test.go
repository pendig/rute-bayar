package xendit

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
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
		_, _ = w.Write([]byte(`{"business_id":"biz_123"}`))
	}))
	defer server.Close()

	adapter := New(WithSecretKey("secret_key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	info, err := adapter.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth returned error: %v", err)
	}
	if info.BusinessID != "biz_123" {
		t.Fatalf("BusinessID = %q, want biz_123", info.BusinessID)
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

