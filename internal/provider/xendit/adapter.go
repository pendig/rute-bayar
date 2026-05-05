package xendit

import (
	"context"
	"errors"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Code() domain.ProviderCode {
	return domain.ProviderXendit
}

func (a *Adapter) Capabilities() []provider.Capability {
	return []provider.Capability{
		{Code: "payment.create", Description: "Create payment through Xendit Payment Sessions", Enabled: true},
		{Code: "payment.status", Description: "Get payment status from Xendit", Enabled: true},
		{Code: "payment.refund", Description: "Refund supported Xendit transactions", Enabled: true},
		{Code: "webhook.verify", Description: "Verify Xendit webhook headers", Enabled: true},
	}
}

func (a *Adapter) CreatePayment(context.Context, provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	return provider.CreatePaymentResponse{}, errors.New("xendit create payment is not implemented yet")
}

func (a *Adapter) GetPaymentStatus(context.Context, string) (domain.PaymentStatus, []byte, error) {
	return "", nil, errors.New("xendit payment status is not implemented yet")
}

func (a *Adapter) RefundPayment(context.Context, provider.RefundRequest) (provider.RefundResponse, error) {
	return provider.RefundResponse{}, errors.New("xendit refund is not implemented yet")
}

func (a *Adapter) VerifyWebhook(context.Context, provider.WebhookRequest) error {
	return errors.New("xendit webhook verification is not implemented yet")
}

func (a *Adapter) ParseWebhook(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, errors.New("xendit webhook parsing is not implemented yet")
}

