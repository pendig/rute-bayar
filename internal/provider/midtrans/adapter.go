package midtrans

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

var midtransTransactionStatusMap = provider.StatusMap{
	"PENDING":        domain.PaymentStatusPending,
	"SETTLEMENT":     domain.PaymentStatusSettled,
	"DENY":           domain.PaymentStatusFailed,
	"FAILURE":        domain.PaymentStatusFailed,
	"CANCEL":         domain.PaymentStatusCancelled,
	"EXPIRE":         domain.PaymentStatusExpired,
	"REFUND":         domain.PaymentStatusRefunded,
	"PARTIAL_REFUND": domain.PaymentStatusPartialRefunded,
}

func MapTransactionStatus(transactionStatus, fraudStatus string) domain.PaymentStatus {
	if strings.EqualFold(strings.TrimSpace(transactionStatus), "capture") {
		if strings.EqualFold(strings.TrimSpace(fraudStatus), "accept") {
			return domain.PaymentStatusCaptured
		}
		return domain.PaymentStatusPending
	}
	return provider.MapPaymentStatus(transactionStatus, midtransTransactionStatusMap, domain.PaymentStatusPending)
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
	paymentType = normalizeMidtransPaymentType(paymentType)
	bankCode := strings.TrimSpace(request.Channel)
	if paymentType != "bank_transfer" && paymentType != "credit_card" && paymentType != "qris" {
		return provider.CreatePaymentResponse{}, fmt.Errorf("midtrans payment method %q is not implemented yet", paymentType)
	}
	if paymentType == "bank_transfer" && bankCode == "" {
		return provider.CreatePaymentResponse{}, errors.New("midtrans bank channel is required for bank transfer")
	}
	cardToken := strings.TrimSpace(request.CardToken)
	if paymentType == "credit_card" && cardToken == "" {
		return provider.CreatePaymentResponse{}, errors.New("midtrans card token is required for credit card")
	}

	payload := midtransCreateChargeRequest{
		PaymentType: paymentType,
		TransactionDetails: midtransTransactionDetails{
			OrderID:     request.ExternalRef,
			GrossAmount: request.Amount,
		},
	}
	if paymentType == "bank_transfer" {
		payload.BankTransfer = &midtransBankTransfer{Bank: bankCode}
	}
	if paymentType == "credit_card" {
		payload.CreditCard = &midtransCreditCard{
			TokenID:        cardToken,
			Authentication: true,
		}
	}
	if paymentType == "qris" {
		payload.QRIS = &midtransQRIS{Acquirer: bankCode}
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
	} else if qrURL := midtransActionURL(parsed.Actions, "generate-qr-code"); qrURL != "" {
		response.RedirectURL = qrURL
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

func (a *Adapter) RefundPayment(ctx context.Context, request provider.RefundRequest) (provider.RefundResponse, error) {
	if a.serverKey == "" {
		return provider.RefundResponse{}, errors.New("midtrans server key is required")
	}

	orderID := strings.TrimSpace(request.ProviderReference)
	if orderID == "" {
		return provider.RefundResponse{}, errors.New("midtrans order id is required")
	}

	refundKey := strings.TrimSpace(request.ReferenceID)
	if refundKey == "" {
		refundKey = orderID + "-refund"
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		reason = "requested by merchant"
	}

	payload := midtransRefundRequest{
		RefundKey: refundKey,
		Reason:    reason,
	}
	if request.Amount > 0 {
		amount := request.Amount
		payload.Amount = &amount
	}

	rawRequest, err := json.Marshal(payload)
	if err != nil {
		return provider.RefundResponse{}, fmt.Errorf("marshal midtrans refund request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v2/%s/refund", a.baseURL, url.PathEscape(orderID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawRequest))
	if err != nil {
		return provider.RefundResponse{}, fmt.Errorf("create midtrans refund request: %w", err)
	}
	req.SetBasicAuth(a.serverKey, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.RefundResponse{ProviderReference: orderID, RawRequestJSON: rawRequest}, fmt.Errorf("call midtrans refund: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.RefundResponse{ProviderReference: orderID, RawRequestJSON: rawRequest}, fmt.Errorf("read midtrans refund response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.RefundResponse{ProviderReference: orderID, RawRequestJSON: rawRequest, RawResponseJSON: rawResponse, Status: domain.PaymentStatusFailed}, fmt.Errorf("midtrans refund returned status %d", resp.StatusCode)
	}

	var parsed midtransRefundResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.RefundResponse{ProviderReference: orderID, RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal midtrans refund response: %w", err)
	}
	if !midtransStatusCodeOK(parsed.StatusCode) {
		return provider.RefundResponse{
			ProviderReference: orderID,
			Status:            domain.PaymentStatusFailed,
			RawRequestJSON:    rawRequest,
			RawResponseJSON:   rawResponse,
		}, fmt.Errorf("midtrans refund returned status_code %s: %s", parsed.StatusCode, parsed.StatusMessage)
	}

	return provider.RefundResponse{
		ProviderReference: orderID,
		Status:            MapTransactionStatus(parsed.TransactionStatus, ""),
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
	}, nil
}

func normalizeMidtransPaymentType(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "card", "credit-card", "credit_card":
		return "credit_card"
	case "qr", "qris":
		return "qris"
	default:
		return strings.TrimSpace(method)
	}
}

func midtransStatusCodeOK(statusCode string) bool {
	statusCode = strings.TrimSpace(statusCode)
	return statusCode == "" || strings.HasPrefix(statusCode, "2")
}

type midtransCreateChargeRequest struct {
	PaymentType        string                     `json:"payment_type"`
	TransactionDetails midtransTransactionDetails `json:"transaction_details"`
	BankTransfer       *midtransBankTransfer      `json:"bank_transfer,omitempty"`
	CreditCard         *midtransCreditCard        `json:"credit_card,omitempty"`
	QRIS               *midtransQRIS              `json:"qris,omitempty"`
	CustomerDetails    *midtransCustomerDetails   `json:"customer_details,omitempty"`
}

type midtransTransactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}

type midtransBankTransfer struct {
	Bank string `json:"bank"`
}

type midtransCreditCard struct {
	TokenID        string `json:"token_id"`
	Authentication bool   `json:"authentication"`
}

type midtransQRIS struct {
	Acquirer string `json:"acquirer,omitempty"`
}

type midtransCustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type midtransRefundRequest struct {
	RefundKey string `json:"refund_key,omitempty"`
	Amount    *int64 `json:"amount,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type midtransRefundResponse struct {
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	RefundKey         string `json:"refund_key"`
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
	Actions     []struct {
		Name   string `json:"name"`
		Method string `json:"method"`
		URL    string `json:"url"`
	} `json:"actions"`
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

func midtransActionURL(actions []struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	URL    string `json:"url"`
}, name string) string {
	for _, action := range actions {
		if strings.EqualFold(strings.TrimSpace(action.Name), name) && strings.TrimSpace(action.URL) != "" {
			return strings.TrimSpace(action.URL)
		}
	}
	return ""
}

func (a *Adapter) VerifyWebhook(_ context.Context, req provider.WebhookRequest) error {
	if a.serverKey == "" {
		return errors.New("midtrans server key is required")
	}

	var payload struct {
		OrderID      any    `json:"order_id"`
		StatusCode   any    `json:"status_code"`
		GrossAmount  any    `json:"gross_amount"`
		SignatureKey string `json:"signature_key"`
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return fmt.Errorf("parse midtrans webhook payload: %w", err)
	}

	orderID := normalizeMidtransField(payload.OrderID)
	statusCode := normalizeMidtransField(payload.StatusCode)
	grossAmount := normalizeMidtransField(payload.GrossAmount)
	signatureKey := strings.TrimSpace(payload.SignatureKey)

	if orderID == "" || statusCode == "" || signatureKey == "" {
		return errors.New("midtrans webhook payload missing required fields")
	}
	if grossAmount == "" {
		return errors.New("midtrans webhook payload missing gross_amount")
	}

	expected := sha512Hex(orderID + statusCode + grossAmount + a.serverKey)
	if !strings.EqualFold(expected, signatureKey) {
		return fmt.Errorf("midtrans webhook signature mismatch")
	}

	return nil
}

func (a *Adapter) ParseWebhook(_ context.Context, req provider.WebhookRequest) (provider.WebhookEvent, error) {
	type midtransWebhook struct {
		OrderID           string `json:"order_id"`
		TransactionID     string `json:"transaction_id"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
		PaymentType       string `json:"payment_type"`
	}

	var webhook midtransWebhook
	if err := json.Unmarshal(req.Body, &webhook); err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse midtrans webhook payload: %w", err)
	}

	eventType := strings.TrimSpace(webhook.TransactionStatus)
	if eventType == "" {
		eventType = "notification"
	}
	reference := strings.TrimSpace(webhook.OrderID)
	if reference == "" {
		reference = strings.TrimSpace(webhook.TransactionID)
	}
	eventKey := provider.BuildWebhookEventID(webhook.OrderID, webhook.TransactionID)
	eventID := ""
	if eventKey != "" {
		eventID = provider.BuildWebhookEventID(eventType, eventKey)
	}

	return provider.WebhookEvent{
		ProviderEventID: eventID,
		EventType:       eventType,
		PaymentRef:      reference,
		Status:          MapTransactionStatus(webhook.TransactionStatus, webhook.FraudStatus),
		RawPayloadJSON:  req.Body,
		RawHeadersJSON:  marshalHeaders(req.Headers),
	}, nil
}

func sha512Hex(value string) string {
	raw := sha512.Sum512([]byte(value))
	return hex.EncodeToString(raw[:])
}

func normalizeMidtransField(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if strings.ContainsAny(fmt.Sprintf("%v", v), ".eE") {
			return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', -1, 64), "0"), ".")
		}
		return strconv.FormatFloat(v, 'f', 0, 64)
	case json.Number:
		return strings.TrimSpace(v.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func marshalHeaders(headers http.Header) []byte {
	raw, err := json.Marshal(headers)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
