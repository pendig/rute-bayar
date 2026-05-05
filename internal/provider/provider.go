package provider

import (
	"context"
	"net/http"

	"github.com/pendig/rute-bayar/internal/domain"
)

type Capability struct {
	Code        string
	Description string
	Enabled     bool
}

type CreatePaymentRequest struct {
	ExternalRef    string
	Amount         int64
	Currency       string
	Method         string
	Channel        string
	CustomerName   string
	CustomerEmail  string
	CustomerPhone  string
	MetadataJSON   []byte
}

type CreatePaymentResponse struct {
	ProviderReference string
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

type RefundRequest struct {
	ProviderReference string
	Amount            int64
	Reason            string
}

type RefundResponse struct {
	ProviderReference string
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

type Adapter interface {
	Code() domain.ProviderCode
	Capabilities() []Capability
	CreatePayment(context.Context, CreatePaymentRequest) (CreatePaymentResponse, error)
	GetPaymentStatus(context.Context, string) (domain.PaymentStatus, []byte, error)
	RefundPayment(context.Context, RefundRequest) (RefundResponse, error)
	VerifyWebhook(context.Context, WebhookRequest) error
	ParseWebhook(context.Context, WebhookRequest) (WebhookEvent, error)
}
