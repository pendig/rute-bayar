package forwardingsvc

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func TestAddListUpdateRemoveForwardingTargets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	svc := New(store)

	id, err := svc.Add(ctx, AddInput{
		Provider: domain.ProviderXendit,
		Name:     " xendit webhook ",
		URL:      " https://example.test/webhook ",
		Enabled:  true,
		Headers: http.Header{
			"Authorization": []string{"Bearer token"},
		},
		EventFilter: map[string]string{
			"event": "payment_session.created",
		},
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: 5,
			Timeout:     8 * time.Second,
			Backoff:     1 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Add returned empty id")
	}

	items, err := svc.List(ctx, ListInput{Provider: domain.ProviderXendit})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List length = %d, want 1", len(items))
	}
	if items[0].Name != "xendit webhook" {
		t.Fatalf("Name = %q, want trimmed value", items[0].Name)
	}
	if items[0].Headers.Get("Authorization") != "Bearer token" {
		t.Fatalf("Headers authorization = %q, want Bearer token", items[0].Headers.Get("Authorization"))
	}

	items, err = svc.List(ctx, ListInput{Provider: domain.ProviderXendit, IncludeDisabled: true})
	if err != nil {
		t.Fatalf("List include disabled returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List include disabled length = %d, want 1", len(items))
	}

	updateName := "xendit webhook v2"
	updateURL := "https://example.test/webhook-v2"
	if err := svc.Update(ctx, UpdateInput{
		ID:             id,
		Name:           updateName,
		URL:            updateURL,
		Enabled:        false,
		EnabledSet:     true,
		Headers:        http.Header{"X-Trace": []string{"trace-1"}},
		HeadersSet:     true,
		EventFilter:    map[string]string{"event": "payment_session.completed"},
		EventFilterSet: true,
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: 7,
			Timeout:     10 * time.Second,
			Backoff:     2 * time.Second,
		},
		RetryPolicySet: true,
	}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	target, err := store.GetForwardingTarget(ctx, id)
	if err != nil {
		t.Fatalf("GetForwardingTarget returned error: %v", err)
	}
	if target.Name != updateName {
		t.Fatalf("updated Name = %q, want %q", target.Name, updateName)
	}
	if target.URL != updateURL {
		t.Fatalf("updated URL = %q, want %q", target.URL, updateURL)
	}
	if target.Enabled {
		t.Fatal("target should be disabled")
	}
	if target.RetryPolicy.MaxAttempts != 7 {
		t.Fatalf("updated MaxAttempts = %d, want 7", target.RetryPolicy.MaxAttempts)
	}

	rawHeaders, err := json.Marshal(target.Headers)
	if err != nil {
		t.Fatalf("marshal headers: %v", err)
	}
	if len(rawHeaders) == 0 {
		t.Fatal("expected headers to be persisted")
	}

	if err := svc.Remove(ctx, id); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := store.GetForwardingTarget(ctx, id); err == nil {
		t.Fatal("GetForwardingTarget returned nil error after delete")
	}
}

func TestAddRejectsInvalidRetryPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	svc := New(store)
	if _, err := svc.Add(ctx, AddInput{
		Provider: domain.ProviderMidtrans,
		Name:     "forward",
		URL:      "https://example.test/webhook",
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: -1,
			Timeout:     5 * time.Second,
			Backoff:     time.Second,
		},
	}); err == nil {
		t.Fatal("Add returned nil error for invalid retry policy")
	}
}

func TestListRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	svc := New(store)
	if _, err := svc.List(ctx, ListInput{Provider: "unknown"}); err == nil {
		t.Fatal("List returned nil error for unknown provider")
	}
}
