package midtrans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

const defaultSandboxBaseURL = "https://api.sandbox.midtrans.com"

type Adapter struct {
	serverKey string
	baseURL   string
	client    *http.Client
}

type Option func(*Adapter)

func New(options ...Option) *Adapter {
	adapter := &Adapter{
		baseURL: defaultSandboxBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithServerKey(serverKey string) Option {
	return func(adapter *Adapter) {
		adapter.serverKey = strings.TrimSpace(serverKey)
	}
}

func WithBaseURL(baseURL string) Option {
	return func(adapter *Adapter) {
		adapter.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(adapter *Adapter) {
		if client != nil {
			adapter.client = client
		}
	}
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

type AuthTestResult struct {
	StatusCode    string
	StatusMessage string
	RawJSON       []byte
}

func (a *Adapter) TestAuth(ctx context.Context) (AuthTestResult, error) {
	if a.serverKey == "" {
		return AuthTestResult{}, errors.New("midtrans server key is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v2/rute-bayar-auth-test/status", nil)
	if err != nil {
		return AuthTestResult{}, fmt.Errorf("create midtrans auth test request: %w", err)
	}
	req.SetBasicAuth(a.serverKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return AuthTestResult{}, fmt.Errorf("call midtrans auth test: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AuthTestResult{}, fmt.Errorf("read midtrans auth test response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return AuthTestResult{RawJSON: body}, fmt.Errorf("midtrans authentication failed with status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AuthTestResult{RawJSON: body}, fmt.Errorf("midtrans auth test returned status %d", resp.StatusCode)
	}

	var payload struct {
		StatusCode    string `json:"status_code"`
		StatusMessage string `json:"status_message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return AuthTestResult{RawJSON: body}, fmt.Errorf("unmarshal midtrans auth test response: %w", err)
	}

	return AuthTestResult{
		StatusCode:    payload.StatusCode,
		StatusMessage: payload.StatusMessage,
		RawJSON:       body,
	}, nil
}

func MapTransactionStatus(transactionStatus, fraudStatus string) domain.PaymentStatus {
	switch strings.ToLower(strings.TrimSpace(transactionStatus)) {
	case "pending":
		return domain.PaymentStatusPending
	case "settlement":
		return domain.PaymentStatusSettled
	case "capture":
		if strings.EqualFold(strings.TrimSpace(fraudStatus), "accept") {
			return domain.PaymentStatusCaptured
		}
		return domain.PaymentStatusPending
	case "deny", "failure":
		return domain.PaymentStatusFailed
	case "cancel":
		return domain.PaymentStatusCancelled
	case "expire":
		return domain.PaymentStatusExpired
	case "refund":
		return domain.PaymentStatusRefunded
	case "partial_refund":
		return domain.PaymentStatusPartialRefunded
	default:
		return domain.PaymentStatusPending
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
