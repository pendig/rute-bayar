package doku

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
)

const (
	defaultSandboxBaseURL    = "https://api-sandbox.doku.com"
	defaultProductionBaseURL = "https://api.doku.com"
	checkoutPaymentPath      = "/checkout/v1/payment"
	orderStatusPathPrefix    = "/orders/v1/status/"
	defaultWebhookTargetPath = "/webhooks/doku"
)

type Adapter struct {
	clientID          string
	secretKey         string
	baseURL           string
	webhookTargetPath string
	client            *http.Client
}

type Option func(*Adapter)

func New(options ...Option) *Adapter {
	adapter := &Adapter{
		baseURL:           defaultSandboxBaseURL,
		webhookTargetPath: defaultWebhookTargetPath,
		client:            &http.Client{Timeout: 15 * time.Second},
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithClientID(clientID string) Option {
	return func(adapter *Adapter) {
		adapter.clientID = strings.TrimSpace(clientID)
	}
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

func WithWebhookTargetPath(path string) Option {
	return func(adapter *Adapter) {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			adapter.webhookTargetPath = ensureLeadingSlash(trimmed)
		}
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
	return domain.ProviderDoku
}

func (a *Adapter) Capabilities() []provider.Capability {
	return []provider.Capability{
		{Code: "payment.create", Description: "Create payment through DOKU Checkout", Enabled: true},
		{Code: "payment.status", Description: "Get payment status from DOKU Check Status API", Enabled: true},
		{Code: "payment.refund", Description: "DOKU refund API requires separate refund/disbursement setup", Enabled: false},
		{Code: "webhook.verify", Description: "Verify DOKU HTTP Notification signature", Enabled: true},
	}
}

type AuthTestResult struct {
	StatusCode int
	RawJSON    []byte
}

func (a *Adapter) TestAuth(ctx context.Context) (AuthTestResult, error) {
	if err := a.validateCredential(); err != nil {
		return AuthTestResult{}, err
	}

	reference := "rute-bayar-auth-test"
	targetPath := orderStatusPathPrefix + url.PathEscape(reference)
	rawRequest := []byte(fmt.Sprintf(`{"reference":"%s"}`, reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+targetPath, nil)
	if err != nil {
		return AuthTestResult{}, fmt.Errorf("create doku auth test request: %w", err)
	}
	a.signRequest(req, targetPath, nil)

	resp, err := a.client.Do(req)
	if err != nil {
		return AuthTestResult{}, fmt.Errorf("call doku auth test: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AuthTestResult{StatusCode: resp.StatusCode, RawJSON: rawRequest}, fmt.Errorf("read doku auth test response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return AuthTestResult{StatusCode: resp.StatusCode, RawJSON: body}, fmt.Errorf("doku authentication failed with status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "invalid_signature") {
		return AuthTestResult{StatusCode: resp.StatusCode, RawJSON: body}, fmt.Errorf("doku authentication failed with invalid signature")
	}
	if resp.StatusCode >= 500 {
		return AuthTestResult{StatusCode: resp.StatusCode, RawJSON: body}, fmt.Errorf("doku auth test returned status %d", resp.StatusCode)
	}
	return AuthTestResult{StatusCode: resp.StatusCode, RawJSON: body}, nil
}

func (a *Adapter) CreatePayment(ctx context.Context, request provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	if err := a.validateCredential(); err != nil {
		return provider.CreatePaymentResponse{}, err
	}
	request.ExternalRef = strings.TrimSpace(request.ExternalRef)
	if request.ExternalRef == "" {
		return provider.CreatePaymentResponse{}, errors.New("doku invoice number is required")
	}
	if request.Amount <= 0 {
		return provider.CreatePaymentResponse{}, errors.New("doku amount must be greater than zero")
	}

	currency := strings.ToUpper(strings.TrimSpace(request.Currency))
	if currency == "" {
		currency = "IDR"
	}
	paymentMethods, err := dokuPaymentMethodTypes(request.Method, request.Channel)
	if err != nil {
		return provider.CreatePaymentResponse{}, err
	}

	payload := dokuCheckoutRequest{
		Order: dokuCheckoutOrder{
			Amount:        request.Amount,
			InvoiceNumber: request.ExternalRef,
			Currency:      currency,
		},
		Payment: dokuCheckoutPayment{
			PaymentDueDate: 60,
			PaymentMethods: paymentMethods,
		},
		Customer: dokuCustomerFromRequest(request),
	}
	if notificationURL := strings.TrimSpace(request.NotificationURL); notificationURL != "" {
		payload.AdditionalInfo.OverrideNotificationURL = notificationURL
		if strings.EqualFold(strings.TrimSpace(request.Method), "ewallet") {
			payload.AdditionalInfo.DokuWalletNotifyURL = notificationURL
		}
	}

	rawRequest, err := json.Marshal(payload)
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("marshal doku create payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+checkoutPaymentPath, bytes.NewReader(rawRequest))
	if err != nil {
		return provider.CreatePaymentResponse{}, fmt.Errorf("create doku create payment request: %w", err)
	}
	a.signRequest(req, checkoutPaymentPath, rawRequest)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest}, fmt.Errorf("call doku create payment: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest}, fmt.Errorf("read doku create payment response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("doku create payment returned status %d", resp.StatusCode)
	}

	var parsed dokuCheckoutResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal doku create payment response: %w", err)
	}

	response := provider.CreatePaymentResponse{
		ProviderReference: firstNonEmpty(parsed.Response.Order.InvoiceNumber, request.ExternalRef),
		PaymentSessionID:  parsed.Response.Order.SessionID,
		TransactionID:     firstNonEmpty(parsed.Response.Order.SessionID, parsed.Response.Headers.RequestID),
		OrderID:           firstNonEmpty(parsed.Response.Order.InvoiceNumber, request.ExternalRef),
		PaymentType:       strings.Join(parsed.Response.Payment.PaymentMethods, ","),
		TransactionStatus: "PENDING",
		ExpiryTime:        parsed.Response.Payment.ExpiredDate,
		RedirectURL:       parsed.Response.Payment.URL,
		Status:            domain.PaymentStatusPending,
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
	}
	return response, nil
}

func (a *Adapter) GetPaymentStatus(ctx context.Context, reference string) (provider.PaymentStatusResponse, error) {
	if err := a.validateCredential(); err != nil {
		return provider.PaymentStatusResponse{}, err
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return provider.PaymentStatusResponse{}, errors.New("doku invoice number or request id is required")
	}

	targetPath := orderStatusPathPrefix + url.PathEscape(reference)
	rawRequest := []byte(fmt.Sprintf(`{"reference":"%s"}`, reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+targetPath, nil)
	if err != nil {
		return provider.PaymentStatusResponse{}, fmt.Errorf("create doku payment status request: %w", err)
	}
	a.signRequest(req, targetPath, nil)
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("call doku payment status: %w", err)
	}
	defer resp.Body.Close()

	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest}, fmt.Errorf("read doku payment status response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("doku payment status returned status %d", resp.StatusCode)
	}

	var parsed dokuStatusResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("unmarshal doku payment status response: %w", err)
	}

	statusText := firstNonEmpty(parsed.Transaction.Status, parsed.Order.Status)
	response := provider.PaymentStatusResponse{
		ProviderReference: firstNonEmpty(parsed.Order.InvoiceNumber, reference),
		TransactionID:     parsed.Transaction.OriginalRequestID,
		OrderID:           firstNonEmpty(parsed.Order.InvoiceNumber, reference),
		PaymentType:       firstNonEmpty(parsed.Channel.ID, parsed.Service.ID),
		StatusCode:        statusText,
		StatusMessage:     firstNonEmpty(parsed.Transaction.Status, parsed.Order.Status),
		TransactionStatus: parsed.Transaction.Status,
		Status:            mapDokuStatus(statusText),
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   rawResponse,
	}
	response.VANumber = parsed.VirtualAccountInfo.VirtualAccountNumber
	return response, nil
}

func (a *Adapter) RefundPayment(_ context.Context, request provider.RefundRequest) (provider.RefundResponse, error) {
	rawRequest, _ := json.Marshal(request)
	return provider.RefundResponse{
		ProviderReference: strings.TrimSpace(request.ProviderReference),
		Status:            domain.PaymentStatusPending,
		RawRequestJSON:    rawRequest,
		RawResponseJSON:   []byte(`{}`),
	}, errors.New("doku refund is not implemented yet; DOKU refunds require Refund API or disbursement setup")
}

func (a *Adapter) VerifyWebhook(_ context.Context, req provider.WebhookRequest) error {
	if err := a.validateCredential(); err != nil {
		return err
	}

	clientID := strings.TrimSpace(req.Headers.Get("Client-Id"))
	requestID := strings.TrimSpace(req.Headers.Get("Request-Id"))
	requestTimestamp := strings.TrimSpace(req.Headers.Get("Request-Timestamp"))
	signature := strings.TrimSpace(req.Headers.Get("Signature"))
	if clientID == "" || requestID == "" || requestTimestamp == "" || signature == "" {
		return errors.New("doku webhook missing required signature headers")
	}
	if clientID != a.clientID {
		return errors.New("doku webhook client id mismatch")
	}

	targetPath := strings.TrimSpace(req.TargetPath)
	if targetPath == "" {
		targetPath = a.webhookTargetPath
	}
	expected := dokuSignature(a.clientID, requestID, requestTimestamp, ensureLeadingSlash(targetPath), dokuDigest(req.Body), a.secretKey)
	if subtle.ConstantTimeCompare([]byte(dokuSignatureValue(signature)), []byte(dokuSignatureValue(expected))) != 1 {
		return errors.New("doku webhook signature mismatch")
	}
	return nil
}

func (a *Adapter) ParseWebhook(_ context.Context, req provider.WebhookRequest) (provider.WebhookEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse doku webhook payload: %w", err)
	}

	order := dokuObjectChild(payload, "order")
	transaction := dokuObjectChild(payload, "transaction")
	verification := dokuObjectChild(payload, "verification")

	reference := firstNonEmpty(
		dokuStringFromObject(order, "invoice_number"),
		dokuStringFromObject(payload, "invoice_number", "order_id"),
	)
	status := firstNonEmpty(
		dokuStringFromObject(transaction, "status"),
		dokuStringFromObject(order, "status"),
		dokuStringFromObject(verification, "status"),
	)
	eventType := status
	if eventType == "" {
		eventType = "notification"
	}
	eventID := provider.BuildWebhookEventID(
		eventType,
		reference,
		dokuStringFromObject(transaction, "original_request_id"),
		req.Headers.Get("Request-Id"),
	)

	return provider.WebhookEvent{
		ProviderEventID: strings.TrimSpace(eventID),
		EventType:       eventType,
		PaymentRef:      reference,
		Status:          mapDokuStatus(status),
		RawPayloadJSON:  req.Body,
		RawHeadersJSON:  marshalHeaders(req.Headers),
	}, nil
}

func (a *Adapter) validateCredential() error {
	if strings.TrimSpace(a.clientID) == "" {
		return errors.New("doku client id is required")
	}
	if strings.TrimSpace(a.secretKey) == "" {
		return errors.New("doku secret key is required")
	}
	if strings.TrimSpace(a.baseURL) == "" {
		return errors.New("doku base url is required")
	}
	return nil
}

func (a *Adapter) signRequest(req *http.Request, targetPath string, body []byte) {
	requestID := newRequestID()
	requestTimestamp := time.Now().UTC().Format(time.RFC3339)
	digest := ""
	if len(body) > 0 {
		digest = dokuDigest(body)
		req.Header.Set("Digest", digest)
	}
	req.Header.Set("Client-Id", a.clientID)
	req.Header.Set("Request-Id", requestID)
	req.Header.Set("Request-Timestamp", requestTimestamp)
	req.Header.Set("Signature", dokuSignature(a.clientID, requestID, requestTimestamp, targetPath, digest, a.secretKey))
}

func dokuDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func dokuSignature(clientID, requestID, requestTimestamp, targetPath, digest, secretKey string) string {
	var component strings.Builder
	component.WriteString("Client-Id:" + clientID)
	component.WriteString("\nRequest-Id:" + requestID)
	component.WriteString("\nRequest-Timestamp:" + requestTimestamp)
	component.WriteString("\nRequest-Target:" + ensureLeadingSlash(targetPath))
	if digest != "" {
		component.WriteString("\nDigest:" + digest)
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(component.String()))
	return "HMACSHA256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func dokuSignatureValue(signature string) string {
	signature = strings.TrimSpace(signature)
	if before, after, found := strings.Cut(signature, "="); found && strings.EqualFold(before, "HMACSHA256") {
		return after
	}
	return signature
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("rutebayar-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func ensureLeadingSlash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

type dokuCheckoutRequest struct {
	Order          dokuCheckoutOrder          `json:"order"`
	Payment        dokuCheckoutPayment        `json:"payment"`
	Customer       *dokuCheckoutCustomer      `json:"customer,omitempty"`
	AdditionalInfo dokuCheckoutAdditionalInfo `json:"additional_info,omitempty"`
}

type dokuCheckoutOrder struct {
	Amount        int64  `json:"amount"`
	InvoiceNumber string `json:"invoice_number"`
	Currency      string `json:"currency,omitempty"`
}

type dokuCheckoutPayment struct {
	PaymentDueDate int      `json:"payment_due_date,omitempty"`
	PaymentMethods []string `json:"payment_method_types,omitempty"`
}

type dokuCheckoutCustomer struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type dokuCheckoutAdditionalInfo struct {
	OverrideNotificationURL string `json:"override_notification_url,omitempty"`
	DokuWalletNotifyURL     string `json:"doku_wallet_notify_url,omitempty"`
}

type dokuCheckoutResponse struct {
	Message  []string `json:"message"`
	Response struct {
		Order struct {
			Amount        any    `json:"amount"`
			InvoiceNumber string `json:"invoice_number"`
			Currency      string `json:"currency"`
			SessionID     string `json:"session_id"`
		} `json:"order"`
		Payment struct {
			PaymentMethods []string `json:"payment_method_types"`
			TokenID        string   `json:"token_id"`
			URL            string   `json:"url"`
			ExpiredDate    string   `json:"expired_date"`
		} `json:"payment"`
		Headers struct {
			RequestID string `json:"request_id"`
			ClientID  string `json:"client_id"`
		} `json:"headers"`
	} `json:"response"`
}

type dokuStatusResponse struct {
	Order struct {
		InvoiceNumber string `json:"invoice_number"`
		Amount        any    `json:"amount"`
		Status        string `json:"status"`
		Date          string `json:"date"`
	} `json:"order"`
	Transaction struct {
		Status            string `json:"status"`
		Date              string `json:"date"`
		OriginalRequestID string `json:"original_request_id"`
	} `json:"transaction"`
	Service struct {
		ID string `json:"id"`
	} `json:"service"`
	Acquirer struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"acquirer"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
	VirtualAccountInfo struct {
		VirtualAccountNumber string `json:"virtual_account_number"`
	} `json:"virtual_account_info"`
	Payment struct {
		URL string `json:"url"`
	} `json:"payment"`
}

func dokuCustomerFromRequest(request provider.CreatePaymentRequest) *dokuCheckoutCustomer {
	customer := dokuCheckoutCustomer{
		Name:  strings.TrimSpace(request.CustomerName),
		Email: strings.TrimSpace(request.CustomerEmail),
		Phone: strings.TrimSpace(request.CustomerPhone),
	}
	if customer.Name == "" && customer.Email == "" && customer.Phone == "" {
		return nil
	}
	return &customer
}

func dokuPaymentMethodTypes(method, channel string) ([]string, error) {
	method = strings.ToLower(strings.TrimSpace(method))
	channel = strings.ToLower(strings.TrimSpace(channel))
	if method == "" || method == "checkout" || method == "payment_link" || method == "payment-link" || method == "all" {
		return nil, nil
	}
	if raw := strings.ToUpper(strings.TrimSpace(method)); strings.HasPrefix(raw, "VIRTUAL_ACCOUNT_") || strings.HasPrefix(raw, "EMONEY_") || raw == "QRIS" || raw == "CREDIT_CARD" {
		return []string{raw}, nil
	}

	switch method {
	case "bank_transfer", "bank-transfer", "va", "virtual_account", "virtual-account":
		code, ok := dokuBankChannelMap[channel]
		if !ok {
			return nil, fmt.Errorf("doku bank channel %q is not supported", channel)
		}
		return []string{code}, nil
	case "qris", "qr":
		return []string{"QRIS"}, nil
	case "card", "credit_card", "credit-card":
		return []string{"CREDIT_CARD"}, nil
	case "ewallet", "e-wallet", "emoney", "e-money":
		code, ok := dokuEWalletChannelMap[channel]
		if !ok {
			return nil, fmt.Errorf("doku e-wallet channel %q is not supported", channel)
		}
		return []string{code}, nil
	case "ovo", "dana", "linkaja", "shopeepay", "shopee_pay", "doku":
		code, ok := dokuEWalletChannelMap[method]
		if !ok {
			return nil, fmt.Errorf("doku e-wallet method %q is not supported", method)
		}
		return []string{code}, nil
	case "convenience_store", "convenience-store", "cstore", "o2o":
		code, ok := dokuO2OChannelMap[channel]
		if !ok {
			return nil, fmt.Errorf("doku convenience store channel %q is not supported", channel)
		}
		return []string{code}, nil
	default:
		return nil, fmt.Errorf("doku payment method %q is not implemented yet", method)
	}
}

var dokuBankChannelMap = map[string]string{
	"bca":                  "VIRTUAL_ACCOUNT_BCA",
	"mandiri":              "VIRTUAL_ACCOUNT_BANK_MANDIRI",
	"bank_mandiri":         "VIRTUAL_ACCOUNT_BANK_MANDIRI",
	"bri":                  "VIRTUAL_ACCOUNT_BRI",
	"bni":                  "VIRTUAL_ACCOUNT_BNI",
	"permata":              "VIRTUAL_ACCOUNT_BANK_PERMATA",
	"cimb":                 "VIRTUAL_ACCOUNT_BANK_CIMB",
	"danamon":              "VIRTUAL_ACCOUNT_BANK_DANAMON",
	"maybank":              "VIRTUAL_ACCOUNT_MAYBANK",
	"btn":                  "VIRTUAL_ACCOUNT_BTN",
	"bnc":                  "VIRTUAL_ACCOUNT_BNC",
	"sinarmas":             "VIRTUAL_ACCOUNT_SINARMAS",
	"doku":                 "VIRTUAL_ACCOUNT_DOKU",
	"syariah_mandiri":      "VIRTUAL_ACCOUNT_BANK_SYARIAH_MANDIRI",
	"bank_syariah_mandiri": "VIRTUAL_ACCOUNT_BANK_SYARIAH_MANDIRI",
}

var dokuEWalletChannelMap = map[string]string{
	"ovo":        "EMONEY_OVO",
	"dana":       "EMONEY_DANA",
	"linkaja":    "EMONEY_LINKAJA",
	"shopeepay":  "EMONEY_SHOPEE_PAY",
	"shopee_pay": "EMONEY_SHOPEE_PAY",
	"doku":       "EMONEY_DOKU",
}

var dokuO2OChannelMap = map[string]string{
	"alfa":      "ONLINE_TO_OFFLINE_ALFA",
	"alfamart":  "ONLINE_TO_OFFLINE_ALFA",
	"indomaret": "ONLINE_TO_OFFLINE_INDOMARET",
}

var dokuStatusMap = provider.StatusMap{
	"ORDER_GENERATE":   domain.PaymentStatusPending,
	"ORDER_GENERATED":  domain.PaymentStatusPending,
	"ORDER_RECOVERED":  domain.PaymentStatusPending,
	"ORDER_EXPIRED":    domain.PaymentStatusExpired,
	"PENDING":          domain.PaymentStatusPending,
	"SUCCESS":          domain.PaymentStatusPaid,
	"FAILED":           domain.PaymentStatusFailed,
	"EXPIRED":          domain.PaymentStatusExpired,
	"REFUNDED":         domain.PaymentStatusRefunded,
	"PARTIAL_REFUNDED": domain.PaymentStatusPartialRefunded,
	"TIMEOUT":          domain.PaymentStatusPending,
	"REDIRECT":         domain.PaymentStatusPending,
	"CANCELLED":        domain.PaymentStatusCancelled,
	"CANCELED":         domain.PaymentStatusCancelled,
	"APPROVE":          domain.PaymentStatusPaid,
	"REJECT":           domain.PaymentStatusFailed,
}

func mapDokuStatus(status string) domain.PaymentStatus {
	return provider.MapPaymentStatus(status, dokuStatusMap, domain.PaymentStatusPending)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dokuObjectChild(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	for actualKey, value := range payload {
		if !strings.EqualFold(actualKey, key) {
			continue
		}
		child, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		return child
	}
	return nil
}

func dokuStringFromObject(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		for actualKey, value := range payload {
			if !strings.EqualFold(actualKey, key) {
				continue
			}
			return normalizeDokuField(value)
		}
	}
	return ""
}

func normalizeDokuField(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
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
