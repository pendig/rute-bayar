package provider

import (
	"context"
	"net/http"
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
)

type Capability struct {
	Code        string
	Description string
	Enabled     bool
}

type CreatePaymentRequest struct {
	ExternalRef   string
	Amount        int64
	Currency      string
	Method        string
	Channel       string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	CardToken     string
	MetadataJSON  []byte
}

type CreatePaymentResponse struct {
	ProviderReference string
	PaymentSessionID  string
	PaymentRequestID  string
	TransactionID     string
	OrderID           string
	PaymentType       string
	TransactionStatus string
	FraudStatus       string
	VANumber          string
	ExpiryTime        string
	RedirectURL       string
	Status            domain.PaymentStatus
	RawRequestJSON    []byte
	RawResponseJSON   []byte
}

type PaymentStatusResponse struct {
	ProviderReference string
	PaymentSessionID  string
	PaymentRequestID  string
	TransactionID     string
	OrderID           string
	PaymentType       string
	StatusCode        string
	StatusMessage     string
	TransactionStatus string
	FraudStatus       string
	VANumber          string
	ExpiryTime        string
	RedirectURL       string
	Status            domain.PaymentStatus
	RawRequestJSON    []byte
	RawResponseJSON   []byte
}

type RefundRequest struct {
	ProviderReference string
	ReferenceID       string
	Amount            int64
	Currency          string
	Reason            string
}

type RefundResponse struct {
	ProviderReference string
	PaymentSessionID  string
	PaymentRequestID  string
	Status            domain.PaymentStatus
	RawRequestJSON    []byte
	RawResponseJSON   []byte
}

type WebhookRequest struct {
	Headers http.Header
	Body    []byte
}

type WebhookEvent struct {
	ProviderEventID string
	EventType       string
	PaymentRef      string
	Status          domain.PaymentStatus
	RawPayloadJSON  []byte
	RawHeadersJSON  []byte
}

// BuildWebhookEventID joins non-empty parts with colon separators.
func BuildWebhookEventID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, ":")
}

type Adapter interface {
	Code() domain.ProviderCode
	Capabilities() []Capability
	CreatePayment(context.Context, CreatePaymentRequest) (CreatePaymentResponse, error)
	GetPaymentStatus(context.Context, string) (PaymentStatusResponse, error)
	RefundPayment(context.Context, RefundRequest) (RefundResponse, error)
	VerifyWebhook(context.Context, WebhookRequest) error
	ParseWebhook(context.Context, WebhookRequest) (WebhookEvent, error)
}
