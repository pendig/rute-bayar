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

const (
	defaultSandboxBaseURL    = "https://api.sandbox.midtrans.com"
	defaultProductionBaseURL = "https://api.midtrans.com"
)

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

func BaseURLForEnvironment(environment domain.Environment) string {
	if environment == domain.EnvironmentProduction {
		return defaultProductionBaseURL
	}
	return defaultSandboxBaseURL
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
	if (resp.StatusCode < 200 || resp.StatusCode >= 300) && resp.StatusCode != http.StatusNotFound {
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

func (a *Adapter) CreatePayment(ctx context.Context, request provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	if a.serverKey == "" {
		return provider.CreatePaymentResponse{}, errors.New("midtrans server key is required")
	}
	if strings.TrimSpace(request.ExternalRef) == "" {
		return provider.CreatePaymentResponse{}, errors.New("midtrans external reference is required")
	}
	if request.Amount <= 0 {
		return provider.CreatePaymentResponse{}, errors.New("midtrans amount must be greater than zero")
	}
	paymentType := strings.TrimSpace(request.Method)
	if paymentType == "" {
		paymentType = "bank_transfer"
	}
	bankCode := strings.TrimSpace(request.Channel)
	if paymentType != "bank_transfer" {
		return provider.CreatePaymentResponse{}, fmt.Errorf("midtrans payment method %q is not implemented yet", paymentType)
	}
	if bankCode == "" {
		return provider.CreatePaymentResponse{}, errors.New("midtrans bank channel is required for bank transfer")
	}

	payload := midtransCreateChargeRequest{
		PaymentType: paymentType,
		TransactionDetails: midtransTransactionDetails{
			OrderID:     request.ExternalRef,
			GrossAmount: request.Amount,
		},
		BankTransfer: &midtransBankTransfer{Bank: bankCode},
	}
	if request.CustomerName != "" || request.CustomerEmail != "" || request.CustomerPhone != "" {
		payload.CustomerDetails = &midtransCustomerDetails{
			FirstName: request.CustomerName,
			Email:     request.CustomerEmail,
			Phone:     request.CustomerPhone,
		}
	}

	rawRequest, err := json.Marshal(payload)
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("marshal midtrans create payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v2/charge", strings.NewReader(string(rawRequest)))
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("create midtrans create payment request: %w", err)
	}
	req.SetBasicAuth(a.serverKey, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("call midtrans create payment: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("read midtrans create payment response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("midtrans create payment returned status %d", resp.StatusCode)
	}

	var parsed midtransChargeResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal midtrans create payment response: %w", err)
	}

	response := provider.CreatePaymentResponse{
		ProviderReference: parsed.OrderID,
		TransactionID:     parsed.TransactionID,
		OrderID:           parsed.OrderID,
		PaymentType:       parsed.PaymentType,
		TransactionStatus: parsed.TransactionStatus,
		FraudStatus:       parsed.FraudStatus,
		ExpiryTime:        parsed.ExpiryTime,
		Status:            MapTransactionStatus(parsed.TransactionStatus, parsed.FraudStatus),
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
	}
	if len(parsed.VANumbers) > 0 {
		response.VANumber = parsed.VANumbers[0].VANumber
	}
	if parsed.RedirectURL != "" {
		response.RedirectURL = parsed.RedirectURL
	}
	return response, nil
}

func (a *Adapter) GetPaymentStatus(ctx context.Context, orderID string) (provider.PaymentStatusResponse, error) {
	if a.serverKey == "" {
		return provider.PaymentStatusResponse{}, errors.New("midtrans server key is required")
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return provider.PaymentStatusResponse{}, errors.New("midtrans order id is required")
	}

	rawRequest := []byte(fmt.Sprintf(`{"order_id":"%s"}`, orderID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v2/"+orderID+"/status", nil)
	if err != nil {
		return provider.PaymentStatusResponse{}, fmt.Errorf("create midtrans payment status request: %w", err)
	}
	req.SetBasicAuth(a.serverKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("call midtrans payment status: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("read midtrans payment status response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("midtrans payment status returned status %d", resp.StatusCode)
	}

	var parsed midtransStatusResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal midtrans payment status response: %w", err)
	}

	response := provider.PaymentStatusResponse{
		ProviderReference: parsed.OrderID,
		TransactionID:     parsed.TransactionID,
		OrderID:           parsed.OrderID,
		PaymentType:       parsed.PaymentType,
		StatusCode:        parsed.StatusCode,
		StatusMessage:     parsed.StatusMessage,
		TransactionStatus: parsed.TransactionStatus,
		FraudStatus:       parsed.FraudStatus,
		ExpiryTime:        parsed.ExpiryTime,
		Status:            MapTransactionStatus(parsed.TransactionStatus, parsed.FraudStatus),
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
	}
	if len(parsed.VANumbers) > 0 {
		response.VANumber = parsed.VANumbers[0].VANumber
	} else if parsed.PermataVANumber != "" {
		response.VANumber = parsed.PermataVANumber
	} else if parsed.BCAVANumber != "" {
		response.VANumber = parsed.BCAVANumber
	}
	if parsed.RedirectURL != "" {
		response.RedirectURL = parsed.RedirectURL
	}
	return response, nil
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

type midtransCreateChargeRequest struct {
	PaymentType        string                    `json:"payment_type"`
	TransactionDetails  midtransTransactionDetails `json:"transaction_details"`
	BankTransfer       *midtransBankTransfer     `json:"bank_transfer,omitempty"`
	CustomerDetails    *midtransCustomerDetails   `json:"customer_details,omitempty"`
}

type midtransTransactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}

type midtransBankTransfer struct {
	Bank string `json:"bank"`
}

type midtransCustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type midtransChargeResponse struct {
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	PaymentType       string `json:"payment_type"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	VANumbers         []struct {
		Bank     string `json:"bank"`
		VANumber string `json:"va_number"`
	} `json:"va_numbers"`
	ExpiryTime  string `json:"expiry_time"`
	RedirectURL string `json:"redirect_url"`
}

type midtransStatusResponse struct {
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	PaymentType       string `json:"payment_type"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	VANumbers         []struct {
		Bank     string `json:"bank"`
		VANumber string `json:"va_number"`
	} `json:"va_numbers"`
	PermataVANumber string `json:"permata_va_number"`
	BCAVANumber     string `json:"bca_va_number"`
	BillKey         string `json:"bill_key"`
	BillerCode      string `json:"biller_code"`
	ExpiryTime      string `json:"expiry_time"`
	RedirectURL     string `json:"redirect_url"`
}
