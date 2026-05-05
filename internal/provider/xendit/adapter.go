package xendit

import (
	"context"
	"crypto/subtle"
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
	secretKey     string
	baseURL       string
	callbackToken string
	client        *http.Client
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

func WithCallbackToken(callbackToken string) Option {
	return func(adapter *Adapter) {
		adapter.callbackToken = strings.TrimSpace(callbackToken)
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
	Balance           *float64
	PermissionWarning string
	RawJSON           []byte
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
		if resp.StatusCode == http.StatusForbidden {
			return AccountInfo{
				PermissionWarning: "authenticated, but the API key cannot read Xendit balance",
				RawJSON:           body,
			}, nil
		}
		return AccountInfo{RawJSON: body}, fmt.Errorf("xendit authentication failed with status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AccountInfo{RawJSON: body}, fmt.Errorf("xendit auth test returned status %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountInfo{RawJSON: body}, fmt.Errorf("unmarshal xendit auth test response: %w", err)
	}

	return AccountInfo{
		Balance: numberFromMap(payload, "balance"),
		RawJSON: body,
	}, nil
}

func (a *Adapter) CreatePayment(context.Context, provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	return provider.CreatePaymentResponse{}, errors.New("xendit create payment is not implemented yet")
}

func (a *Adapter) GetPaymentStatus(ctx context.Context, sessionID string) (provider.PaymentStatusResponse, error) {
	if a.secretKey == "" {
		return provider.PaymentStatusResponse{}, errors.New("xendit secret key is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return provider.PaymentStatusResponse{}, errors.New("xendit session id is required")
	}

	rawRequest := []byte(fmt.Sprintf(`{"id":"%s"}`, sessionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/sessions/"+sessionID, nil)
	if err != nil {
		return provider.PaymentStatusResponse{}, fmt.Errorf("create xendit payment status request: %w", err)
	}
	req.SetBasicAuth(a.secretKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("call xendit payment status: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("read xendit payment status response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("xendit payment status returned status %d", resp.StatusCode)
	}

	var parsed xenditSessionResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal xendit payment status response: %w", err)
	}

	response := provider.PaymentStatusResponse{
		ProviderReference: parsed.ID,
		OrderID:           parsed.ReferenceID,
		PaymentType:       parsed.Mode,
		StatusMessage:     parsed.Status,
		Status:            mapXenditSessionStatus(parsed.Status),
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
		RedirectURL:       parsed.PaymentLinkURL,
	}
	return response, nil
}

func (a *Adapter) RefundPayment(context.Context, provider.RefundRequest) (provider.RefundResponse, error) {
	return provider.RefundResponse{}, errors.New("xendit refund is not implemented yet")
}

func numberFromMap(payload map[string]any, key string) *float64 {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	return &number
}

type xenditSessionResponse struct {
	ID             string `json:"id"`
	ReferenceID    string `json:"reference_id"`
	Mode           string `json:"mode"`
	Status         string `json:"status"`
	PaymentLinkURL string `json:"payment_link_url"`
}

func (a *Adapter) VerifyWebhook(_ context.Context, req provider.WebhookRequest) error {
	if a.callbackToken == "" {
		return nil
	}

	headerToken := req.Headers.Get("x-callback-token")
	if headerToken == "" {
		return errors.New("xendit webhook callback token header is missing")
	}
	if subtle.ConstantTimeCompare([]byte(headerToken), []byte(a.callbackToken)) != 1 {
		return errors.New("xendit webhook callback token mismatch")
	}
	return nil
}

func (a *Adapter) ParseWebhook(_ context.Context, req provider.WebhookRequest) (provider.WebhookEvent, error) {
	var payload struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Event      string `json:"event"`
		Reference  string `json:"reference_id"`
		ExternalID string `json:"external_id"`
		OrderID    string `json:"order_id"`
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse xendit webhook payload: %w", err)
	}

	eventType := strings.TrimSpace(payload.Event)
	if eventType == "" {
		eventType = strings.TrimSpace(payload.Status)
	}
	if eventType == "" {
		eventType = "notification"
	}

	reference := firstNonEmpty(payload.Reference, payload.ExternalID, payload.OrderID)
	if reference == "" {
		reference = payload.ID
	}

	return provider.WebhookEvent{
		ProviderEventID: strings.TrimSpace(payload.ID),
		EventType:       eventType,
		PaymentRef:      strings.TrimSpace(reference),
		Status:          mapXenditSessionStatus(payload.Status),
		RawPayloadJSON:  req.Body,
		RawHeadersJSON:  marshalHeaders(req.Headers),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func marshalHeaders(headers http.Header) []byte {
	raw, err := json.Marshal(headers)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func mapXenditSessionStatus(status string) domain.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE":
		return domain.PaymentStatusPending
	case "COMPLETED":
		return domain.PaymentStatusSettled
	case "EXPIRED":
		return domain.PaymentStatusExpired
	case "CANCELLED":
		return domain.PaymentStatusCancelled
	case "FAILED":
		return domain.PaymentStatusFailed
	default:
		return domain.PaymentStatusPending
	}
}
