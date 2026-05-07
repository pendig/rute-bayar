package provider

import (
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
)

type StatusMap map[string]domain.PaymentStatus

func MapPaymentStatus(raw string, statuses StatusMap, fallback domain.PaymentStatus) domain.PaymentStatus {
	status, ok := statuses[NormalizeStatusKey(raw)]
	if !ok {
		return fallback
	}
	return status
}

func NormalizeStatusKey(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}
