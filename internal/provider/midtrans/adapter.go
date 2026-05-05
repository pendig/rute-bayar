package midtrans

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
	return domain.ProviderMidtrans
}

func (a *Adapter) Capabilities() []provider.Capability {
	return []provider.Capability{
		{Code: "payment.create", Description: "Create payment through Midtrans", Enabled: true},
		{Code: "payment.status", Description: "Get transaction status from Midtrans", Enabled: true},
		{Code: "payment.refund", Description: "Refund supported Midtrans transactions", Enabled: true},
		{Code: "webhook.verify", Description: "Verify Midtrans notification signature", Enabled: true},
	}
}

func (a *Adapter) CreatePayment(context.Context, provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	return provider.CreatePaymentResponse{}, errors.New("midtrans create payment is not implemented yet")
}

func (a *Adapter) GetPaymentStatus(context.Context, string) (domain.PaymentStatus, []byte, error) {
	return "", nil, errors.New("midtrans payment status is not implemented yet")
}

func (a *Adapter) RefundPayment(context.Context, provider.RefundRequest) (provider.RefundResponse, error) {
	return provider.RefundResponse{}, errors.New("midtrans refund is not implemented yet")
}

func (a *Adapter) VerifyWebhook(context.Context, provider.WebhookRequest) error {
	return errors.New("midtrans webhook verification is not implemented yet")
}

func (a *Adapter) ParseWebhook(context.Context, provider.WebhookRequest) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, errors.New("midtrans webhook parsing is not implemented yet")
}

