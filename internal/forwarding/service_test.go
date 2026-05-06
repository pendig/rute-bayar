package forwarding

import (
	"net/http"
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
)

func TestEventFilterMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    map[string]any
		headers  http.Header
		filter   map[string]string
		expected bool
	}{
		{
			name:     "no_filter_always_match",
			event:    map[string]any{"event": "payment_session.created"},
			filter:   map[string]string{},
			expected: true,
		},
		{
			name:     "header_match",
			event:    map[string]any{"event": "payment_session.created"},
			headers:  http.Header{"X-Event-Type": []string{"payment_session.created"}},
			filter:   map[string]string{"X-Event-Type": "payment_session.created"},
			expected: true,
		},
		{
			name:     "header_mismatch",
			event:    map[string]any{"event": "payment_session.created"},
			headers:  http.Header{"X-Event-Type": []string{"payment_session.failed"}},
			filter:   map[string]string{"X-Event-Type": "payment_session.created"},
			expected: false,
		},
		{
			name:     "body_match",
			event:    map[string]any{"event": "payment_session.created", "provider": "midtrans"},
			filter:   map[string]string{"event": "payment_session.created"},
			expected: true,
		},
		{
			name:     "body_provider_mismatch",
			event:    map[string]any{"event": "payment_session.created", "provider": "xendit"},
			filter:   map[string]string{"event": "payment_session.created", "provider": "midtrans"},
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := eventFilterMatch(tc.filter, tc.headers, tc.event); got != tc.expected {
				t.Fatalf("eventFilterMatch(%v, %v, %v) = %v, want %v", tc.filter, tc.headers, tc.event, got, tc.expected)
			}
		})
	}
}

func TestDecodePayloadObject(t *testing.T) {
	t.Parallel()

	payload := decodePayloadObject([]byte(`{"event":"payment_session.created","amount":100}`))
	if payload == nil {
		t.Fatal("decodePayloadObject returned nil for valid JSON")
	}
	if got, ok := payload["event"].(string); !ok || got != "payment_session.created" {
		t.Fatalf("payload[event] = %v, want payment_session.created", payload["event"])
	}
}

func TestScalarToString(t *testing.T) {
	t.Parallel()

	if got := scalarToString(123); got != "123" {
		t.Fatalf("scalarToString(123) = %q, want 123", got)
	}
	if got := scalarToString(float64(1000)); got != "1000" {
		t.Fatalf("scalarToString(float64(1000)) = %q, want 1000", got)
	}
	if got := scalarToString(nil); got != "" {
		t.Fatalf("scalarToString(nil) = %q, want empty", got)
	}

	inbound := []byte(`{"count":10}`)
	if _, ok := decodePayloadObject(inbound)["count"]; !ok {
		t.Fatalf("decodePayloadObject expected count field")
	}

	if got := scalarToString(domain.PaymentStatusPending); got != "pending" {
		t.Fatalf("scalarToString(PaymentStatusPending) = %q, want pending", got)
	}
}
