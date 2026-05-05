package xendit

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

const defaultBaseURL = "https://api.xendit.co"

type Adapter struct {
	secretKey string
	baseURL   string
	client    *http.Client
}

type Option func(*Adapter)

func New(options ...Option) *Adapter {
	adapter := &Adapter{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithSecretKey(secretKey string) Option {
	return func(adapter *Adapter) {
		adapter.secretKey = strings.TrimSpace(secretKey)
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

type AccountInfo struct {
	ID          string
	BusinessID  string
	Description string
	RawJSON     []byte
}

func (a *Adapter) TestAuth(ctx context.Context) (AccountInfo, error) {
	if a.secretKey == "" {
		return AccountInfo{}, errors.New("xendit secret key is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/balance", nil)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("create xendit auth test request: %w", err)
	}
	req.SetBasicAuth(a.secretKey, "")

	resp, err := a.client.Do(req)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("call xendit auth test: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("read xendit auth test response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return AccountInfo{RawJSON: body}, fmt.Errorf("xendit authentication failed with status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AccountInfo{RawJSON: body}, fmt.Errorf("xendit auth test returned status %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountInfo{RawJSON: body}, nil
	}

	return AccountInfo{
		ID:          stringFromMap(payload, "id"),
		BusinessID:  stringFromMap(payload, "business_id"),
		Description: stringFromMap(payload, "description"),
		RawJSON:     body,
	}, nil
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

func stringFromMap(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}
