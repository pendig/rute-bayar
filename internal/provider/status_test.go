package provider

import (
	"testing"

	"github.com/pendig/rute-bayar/internal/domain"
)

func TestMapPaymentStatusNormalizesProviderStatus(t *testing.T) {
	t.Parallel()

	statuses := StatusMap{
		"SETTLED": domain.PaymentStatusSettled,
	}
	if got := MapPaymentStatus(" settled ", statuses, domain.PaymentStatusPending); got != domain.PaymentStatusSettled {
		t.Fatalf("MapPaymentStatus normalized = %q, want %q", got, domain.PaymentStatusSettled)
	}
	if got := MapPaymentStatus("unknown", statuses, domain.PaymentStatusPending); got != domain.PaymentStatusPending {
		t.Fatalf("MapPaymentStatus fallback = %q, want %q", got, domain.PaymentStatusPending)
	}
}
