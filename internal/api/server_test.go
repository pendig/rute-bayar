package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRequestIDUnique(t *testing.T) {
	first := newRequestID()
	second := newRequestID()
	if first == second {
		t.Fatalf("expected unique request IDs, got duplicate %q", first)
	}
}

func TestDecodeJSONBody_EnforcesStrictJSONAndSizeLimit(t *testing.T) {
	t.Run("rejects trailing json", func(t *testing.T) {
		payload := struct {
			Name string `json:"name"`
		}{}
		req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"name":"ok"}junk`))
		err := decodeJSONBody(req, &payload)
		if err == nil {
			t.Fatalf("expected strict JSON decode error, got nil")
		}
		apiErr, ok := err.(*apiError)
		if !ok {
			t.Fatalf("expected apiError, got %T", err)
		}
		if apiErr.Code != errBadRequest {
			t.Fatalf("expected error code %q, got %q", errBadRequest, apiErr.Code)
		}
	})

	t.Run("accepts exact json with whitespace", func(t *testing.T) {
		payload := struct {
			Name string `json:"name"`
		}{}
		req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"name":"ok"}  `))
		if err := decodeJSONBody(req, &payload); err != nil {
			t.Fatalf("expected valid JSON body, got %v", err)
		}
		if payload.Name != "ok" {
			t.Fatalf("unexpected payload value: %q", payload.Name)
		}
	})

	t.Run("rejects oversized body", func(t *testing.T) {
		extra := strings.Repeat("a", maxJSONBodyBytes)
		body := `{"payload":"` + extra + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(body))
		payload := struct {
			Payload string `json:"payload"`
		}{}
		if err := decodeJSONBody(req, &payload); err == nil {
			t.Fatal("expected oversized body error")
		}
	})
}

func TestAllowRate_UsesRemoteAddrForInvalidAPIKey(t *testing.T) {
	server := NewServer(Config{APIKey: "expected-key", RateLimitPerMinute: 2})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "198.51.100.7:54321"
	req.Header.Set("X-API-Key", "wrong-key")

	if !server.allowRate(req, req.RemoteAddr) {
		t.Fatal("expected first request to pass")
	}
	if !server.allowRate(req, req.RemoteAddr) {
		t.Fatal("expected second request to pass")
	}
	if server.allowRate(req, req.RemoteAddr) {
		t.Fatal("expected third request to be rate-limited")
	}
}

func TestAllowRate_SeparatesKeysByValidAPIKey(t *testing.T) {
	server := NewServer(Config{APIKey: "expected-key", RateLimitPerMinute: 1})
	validReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	validReq.Header.Set("X-API-Key", "expected-key")
	validReq.RemoteAddr = "198.51.100.7:54321"
	if !server.allowRate(validReq, validReq.RemoteAddr) {
		t.Fatal("expected first request with valid API key to pass")
	}
	if server.allowRate(validReq, validReq.RemoteAddr) {
		t.Fatal("expected second request with same API key to be limited")
	}
}

func TestPruneRateBucketsExpiresStaleBuckets(t *testing.T) {
	server := NewServer(Config{})
	now := time.Now().UTC()
	server.rateBuckets["stale"] = &rateBucket{
		start:  now.Truncate(time.Minute),
		count:  1,
		seenAt: now.Add(-rateBucketTTL - time.Minute),
	}
	server.rateBuckets["active"] = &rateBucket{
		start:  now.Truncate(time.Minute),
		count:  1,
		seenAt: now,
	}

	server.pruneRateBuckets(now)
	if _, ok := server.rateBuckets["stale"]; ok {
		t.Fatal("expected stale rate bucket to be pruned")
	}
	if _, ok := server.rateBuckets["active"]; !ok {
		t.Fatal("expected active rate bucket to remain")
	}
}

func TestPruneIdempotencyExpiresEntries(t *testing.T) {
	server := NewServer(Config{})
	server.idempotencyMap["active"] = idempotencyEntry{
		payload: map[string]string{"status": "active"},
		expires: time.Now().UTC().Add(time.Minute),
	}
	server.idempotencyMap["expired"] = idempotencyEntry{
		payload: map[string]string{"status": "expired"},
		expires: time.Now().UTC().Add(-time.Minute),
	}

	server.pruneIdempotency(time.Now().UTC())
	if _, ok := server.idempotencyMap["expired"]; ok {
		t.Fatal("expected expired idempotency entry to be removed")
	}
	if _, ok := server.idempotencyMap["active"]; !ok {
		t.Fatal("expected active idempotency entry to remain")
	}
}

func TestWriteAndReadIdempotency(t *testing.T) {
	server := NewServer(Config{})
	server.writeIdempotent("cache-key", map[string]any{"status": "ok"})

	cached, ok := server.readIdempotent("cache-key")
	if !ok {
		t.Fatal("expected cached idempotent response")
	}
	cachedMap, ok := cached.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any payload, got %T", cached)
	}
	if got, expected := cachedMap["status"], "ok"; got != expected {
		t.Fatalf("unexpected payload value: %v", got)
	}
}

func TestWriteAndReadIdempotency_ExpiredEntryIsDeleted(t *testing.T) {
	server := NewServer(Config{})
	server.idempotencyMap["cache-key"] = idempotencyEntry{
		payload: map[string]any{"status": "stale"},
		expires: time.Now().UTC().Add(-time.Minute),
	}

	if _, ok := server.readIdempotent("cache-key"); ok {
		t.Fatal("expected expired idempotency entry to be ignored")
	}
	if _, ok := server.idempotencyMap["cache-key"]; ok {
		t.Fatal("expected expired idempotency entry to be removed from cache")
	}
}

func TestServerHandler_OptionsPreflightReturnsNoContentWithCorsHeaders(t *testing.T) {
	server := NewServer(Config{AllowedOrigins: "https://ui.example.com"})
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/provider-accounts", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected OPTIONS preflight status %d, got %d", http.StatusNoContent, res.Code)
	}

	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.com" {
		t.Fatalf("expected allow origin %q, got %q", "https://ui.example.com", got)
	}

	if got := res.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "OPTIONS") {
		t.Fatalf("expected allow methods to include OPTIONS, got %q", got)
	}
}

func TestServerHandler_OptionsPreflightSkipsAuth(t *testing.T) {
	server := NewServer(Config{APIKey: "secret"})
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/payments", nil)
	req.Header.Set("X-API-Key", "wrong")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected OPTIONS preflight status %d, got %d", http.StatusNoContent, res.Code)
	}
}

func TestServerWrap_AuthAndRateLimitForHandlers(t *testing.T) {
	server := NewServer(Config{
		APIKey:             "secret",
		RateLimitPerMinute: 1,
	})
	handler := server.wrap(func(_ *http.Request) (any, error) {
		return map[string]any{"ok": true}, nil
	}, true)

	unauthReq := httptest.NewRequest(http.MethodGet, "/api", nil)
	unauthReq.RemoteAddr = "10.0.0.1:1111"
	unauthReq.Header.Set("X-API-Key", "wrong")
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauthReq)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", unauthRes.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api", nil)
	authReq.RemoteAddr = "10.0.0.2:2222"
	authReq.Header.Set("X-API-Key", "secret")
	authRes := httptest.NewRecorder()
	handler.ServeHTTP(authRes, authReq)
	if authRes.Code != http.StatusOK {
		t.Fatalf("expected first authenticated request to pass, got %d", authRes.Code)
	}
	var firstPayload map[string]any
	if err := json.NewDecoder(authRes.Body).Decode(&firstPayload); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if _, exists := firstPayload["data"]; !exists {
		t.Fatal("expected payload key \"data\"")
	}
	if got := authRes.Header().Get(requestIDHeader); got == "" {
		t.Fatal("expected request ID response header")
	}

	authReq2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	authReq2.RemoteAddr = "10.0.0.2:2222"
	authReq2.Header.Set("X-API-Key", "secret")
	authRes2 := httptest.NewRecorder()
	handler.ServeHTTP(authRes2, authReq2)
	if authRes2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate limit rejection on second authenticated request, got %d", authRes2.Code)
	}
}
