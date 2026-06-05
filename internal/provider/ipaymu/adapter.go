package ipaymu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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
	defaultSandboxBaseURL    = "https://sandbox.ipaymu.com"
	defaultProductionBaseURL = "https://my.ipaymu.com"
)

type Adapter struct {
	va        string
	apiKey    string
	account   string
	baseURL   string
	client    *http.Client
	timestamp func() time.Time
}

type Option func(*Adapter)

func New(options ...Option) *Adapter {
	adapter := &Adapter{baseURL: defaultSandboxBaseURL, client: &http.Client{Timeout: 15 * time.Second}, timestamp: time.Now}
	for _, option := range options {
		option(adapter)
	}
	if adapter.account == "" {
		adapter.account = adapter.va
	}
	return adapter
}

func WithVA(va string) Option { return func(a *Adapter) { a.va = strings.TrimSpace(va) } }
func WithAPIKey(apiKey string) Option {
	return func(a *Adapter) { a.apiKey = strings.TrimSpace(apiKey) }
}
func WithAccount(account string) Option {
	return func(a *Adapter) { a.account = strings.TrimSpace(account) }
}
func WithBaseURL(baseURL string) Option {
	return func(a *Adapter) { a.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/") }
}
func WithHTTPClient(client *http.Client) Option {
	return func(a *Adapter) {
		if client != nil {
			a.client = client
		}
	}
}
func WithTimestamp(fn func() time.Time) Option {
	return func(a *Adapter) {
		if fn != nil {
			a.timestamp = fn
		}
	}
}

func BaseURLForEnvironment(environment domain.Environment) string {
	if environment == domain.EnvironmentProduction {
		return defaultProductionBaseURL
	}
	return defaultSandboxBaseURL
}

func (a *Adapter) Code() domain.ProviderCode { return domain.ProviderIPaymu }

func (a *Adapter) Capabilities() []provider.Capability {
	return []provider.Capability{
		{Code: "payment.create", Description: "Create payment through iPaymu hosted payment page/direct payment", Enabled: true},
		{Code: "payment.status", Description: "Get transaction status from iPaymu", Enabled: true},
		{Code: "payment.refund", Description: "iPaymu refund is not implemented yet", Enabled: false},
		{Code: "webhook.verify", Description: "Verify iPaymu webhook signature when X-Signature headers are present", Enabled: true},
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
	values := url.Values{}
	var payload struct {
		Status  int    `json:"Status"`
		Message string `json:"Message"`
	}
	status, raw, err := a.doForm(ctx, http.MethodGet, "/api/v2/payment-channels", values, &payload)
	if err != nil {
		return AuthTestResult{StatusCode: status, RawJSON: raw}, err
	}
	if status < 200 || status >= 300 || payload.Status >= 400 {
		return AuthTestResult{StatusCode: status, RawJSON: raw}, fmt.Errorf("ipaymu auth probe returned status %d", firstNonZero(payload.Status, status))
	}
	return AuthTestResult{StatusCode: status, RawJSON: raw}, nil
}

func (a *Adapter) CreatePayment(ctx context.Context, request provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	if err := a.validateCredential(); err != nil {
		return provider.CreatePaymentResponse{}, err
	}
	if strings.TrimSpace(request.ExternalRef) == "" {
		return provider.CreatePaymentResponse{}, errors.New("ipaymu reference is required")
	}
	if request.Amount <= 0 {
		return provider.CreatePaymentResponse{}, errors.New("ipaymu amount must be greater than zero")
	}

	method := strings.ToLower(strings.TrimSpace(request.Method))
	if method == "" || method == "redirect" || method == "checkout" || method == "payment" {
		return a.createRedirectPayment(ctx, request)
	}
	return a.createDirectPayment(ctx, request, method)
}

func (a *Adapter) createRedirectPayment(ctx context.Context, request provider.CreatePaymentRequest) (provider.CreatePaymentResponse, error) {
	values := url.Values{}
	values.Add("product[]", firstNonEmpty(request.ExternalRef, "Rute Bayar Payment"))
	values.Add("qty[]", "1")
	values.Add("price[]", strconv.FormatInt(request.Amount, 10))
	values.Set("referenceId", request.ExternalRef)
	if request.NotificationURL != "" {
		values.Set("notifyUrl", request.NotificationURL)
	}
	if request.CustomerName != "" {
		values.Set("buyerName", request.CustomerName)
	}
	if request.CustomerEmail != "" {
		values.Set("buyerEmail", request.CustomerEmail)
	}
	if request.CustomerPhone != "" {
		values.Set("buyerPhone", request.CustomerPhone)
	}
	values.Set("feeDirection", "MERCHANT")
	values.Set("account", a.account)
	values.Set("lang", "id")

	var parsed redirectPaymentResponse
	statusCode, rawResponse, err := a.doForm(ctx, http.MethodPost, "/api/v2/payment", values, &parsed)
	rawRequest, _ := json.Marshal(formSignaturePayload(values))
	if err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, err
	}
	if statusCode < 200 || statusCode >= 300 || parsed.Status >= 400 {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("ipaymu create payment returned status %d", statusCode)
	}
	return provider.CreatePaymentResponse{ProviderReference: firstNonEmpty(parsed.Data.SessionID, request.ExternalRef), PaymentSessionID: parsed.Data.SessionID, TransactionID: parsed.Data.SessionID, OrderID: request.ExternalRef, PaymentType: "redirect", TransactionStatus: "pending", RedirectURL: parsed.Data.URL, Status: domain.PaymentStatusPending, RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, nil
}

func (a *Adapter) createDirectPayment(ctx context.Context, request provider.CreatePaymentRequest, method string) (provider.CreatePaymentResponse, error) {
	if strings.TrimSpace(request.CustomerPhone) == "" {
		return provider.CreatePaymentResponse{}, errors.New("ipaymu direct payment requires customer phone")
	}
	if strings.TrimSpace(request.CustomerEmail) == "" {
		return provider.CreatePaymentResponse{}, errors.New("ipaymu direct payment requires customer email")
	}
	values := url.Values{}
	values.Set("name", firstNonEmpty(request.CustomerName, "Rute Bayar Customer"))
	values.Set("phone", request.CustomerPhone)
	values.Set("email", request.CustomerEmail)
	values.Set("amount", strconv.FormatInt(request.Amount, 10))
	if request.NotificationURL != "" {
		values.Set("notifyUrl", request.NotificationURL)
	}
	values.Set("referenceId", request.ExternalRef)
	values.Set("paymentMethod", method)
	if request.Channel != "" {
		values.Set("paymentChannel", strings.ToLower(request.Channel))
	}
	values.Set("feeDirection", "MERCHANT")
	values.Set("escrow", "0")
	values.Add("product[]", request.ExternalRef)
	values.Add("qty[]", "1")
	values.Add("price[]", strconv.FormatInt(request.Amount, 10))
	values.Set("account", a.account)

	var parsed directPaymentResponse
	statusCode, rawResponse, err := a.doForm(ctx, http.MethodPost, "/api/v2/payment/direct", values, &parsed)
	rawRequest, _ := json.Marshal(formSignaturePayload(values))
	if err != nil {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, err
	}
	if statusCode < 200 || statusCode >= 300 || parsed.Status >= 400 {
		return provider.CreatePaymentResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("ipaymu direct payment returned status %d", statusCode)
	}
	return provider.CreatePaymentResponse{ProviderReference: strconv.FormatInt(parsed.Data.TransactionID, 10), PaymentSessionID: parsed.Data.SessionID, TransactionID: strconv.FormatInt(parsed.Data.TransactionID, 10), OrderID: firstNonEmpty(parsed.Data.ReferenceID, request.ExternalRef), PaymentType: firstNonEmpty(parsed.Data.Via, method), TransactionStatus: "pending", VANumber: parsed.Data.PaymentNo, ExpiryTime: parsed.Data.Expired, RedirectURL: firstNonEmpty(parsed.Data.QrImage, parsed.Data.QrTemplate), Status: domain.PaymentStatusPending, RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, nil
}

func (a *Adapter) GetPaymentStatus(ctx context.Context, reference string) (provider.PaymentStatusResponse, error) {
	if err := a.validateCredential(); err != nil {
		return provider.PaymentStatusResponse{}, err
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return provider.PaymentStatusResponse{}, errors.New("ipaymu transaction id is required")
	}
	values := url.Values{}
	values.Set("transactionId", reference)
	values.Set("account", a.account)
	var parsed transactionResponse
	statusCode, rawResponse, err := a.doForm(ctx, http.MethodPost, "/api/v2/transaction", values, &parsed)
	rawRequest, _ := json.Marshal(formSignaturePayload(values))
	if err != nil {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, err
	}
	if statusCode < 200 || statusCode >= 300 || parsed.Status >= 400 {
		return provider.PaymentStatusResponse{RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, fmt.Errorf("ipaymu payment status returned status %d", statusCode)
	}
	return provider.PaymentStatusResponse{ProviderReference: strconv.FormatInt(parsed.Data.TransactionID, 10), PaymentSessionID: parsed.Data.SessionID, TransactionID: strconv.FormatInt(parsed.Data.TransactionID, 10), OrderID: parsed.Data.ReferenceID, PaymentType: parsed.Data.TypeDesc, StatusCode: strconv.Itoa(parsed.Data.Status), StatusMessage: parsed.Data.StatusDesc, TransactionStatus: parsed.Data.PaidStatus, VANumber: parsed.Data.PaymentCode, Status: MapStatusCode(parsed.Data.Status), RawRequestJSON: rawRequest, RawResponseJSON: rawResponse}, nil
}

func (a *Adapter) RefundPayment(context.Context, provider.RefundRequest) (provider.RefundResponse, error) {
	return provider.RefundResponse{}, errors.New("ipaymu refund is not implemented yet")
}

func (a *Adapter) VerifyWebhook(_ context.Context, request provider.WebhookRequest) error {
	sig := strings.TrimSpace(firstNonEmpty(request.Headers.Get("X-Signature"), request.Headers.Get("signature")))
	if sig == "" {
		return errors.New("ipaymu webhook signature is missing")
	}
	timestamp := strings.TrimSpace(request.Headers.Get("X-Timestamp"))
	externalID := strings.TrimSpace(request.Headers.Get("X-External-ID"))
	expected := GenerateWebhookSignature(a.apiKey, timestamp, externalID, request.Body)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(expected)) != 1 {
		return errors.New("ipaymu webhook signature mismatch")
	}
	return nil
}

func (a *Adapter) ParseWebhook(_ context.Context, request provider.WebhookRequest) (provider.WebhookEvent, error) {
	values, err := url.ParseQuery(string(request.Body))
	if err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("parse ipaymu webhook form: %w", err)
	}
	statusCodeValue := strings.TrimSpace(values.Get("status_code"))
	if statusCodeValue == "" {
		return provider.WebhookEvent{}, errors.New("ipaymu webhook missing status_code")
	}
	statusCode, err := strconv.Atoi(statusCodeValue)
	if err != nil {
		return provider.WebhookEvent{}, fmt.Errorf("ipaymu webhook invalid status_code: %w", err)
	}
	trxID := strings.TrimSpace(values.Get("trx_id"))
	ref := strings.TrimSpace(values.Get("reference_id"))
	if trxID == "" && ref == "" {
		return provider.WebhookEvent{}, errors.New("ipaymu webhook missing trx_id or reference_id")
	}
	headersJSON, _ := json.Marshal(request.Headers)
	return provider.WebhookEvent{ProviderEventID: provider.BuildWebhookEventID("ipaymu", trxID, ref, statusCodeValue), EventType: firstNonEmpty(values.Get("status"), "payment.status"), PaymentRef: firstNonEmpty(ref, trxID), Status: MapStatusCode(statusCode), RawPayloadJSON: request.Body, RawHeadersJSON: headersJSON}, nil
}

func MapStatusCode(code int) domain.PaymentStatus {
	switch code {
	case -2:
		return domain.PaymentStatusExpired
	case 0:
		return domain.PaymentStatusPending
	case 1, 6, 7:
		return domain.PaymentStatusPaid
	case 3:
		return domain.PaymentStatusRefunded
	case 2, 4, 5:
		return domain.PaymentStatusFailed
	default:
		return domain.PaymentStatusPending
	}
}

func (a *Adapter) validateCredential() error {
	if a.va == "" {
		return errors.New("ipaymu va is required")
	}
	if a.apiKey == "" {
		return errors.New("ipaymu api key is required")
	}
	if a.account == "" {
		a.account = a.va
	}
	return nil
}

func (a *Adapter) doForm(ctx context.Context, method, path string, values url.Values, out any) (int, []byte, error) {
	var payload any
	var body io.Reader = http.NoBody
	if method == http.MethodGet {
		body = http.NoBody
	} else {
		payload = formSignaturePayload(values)
		rawBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal ipaymu request body: %w", err)
		}
		body = strings.NewReader(string(rawBody))
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	a.signRequest(req, method, payload)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, raw, err
	}
	if len(raw) > 0 && out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, raw, fmt.Errorf("unmarshal ipaymu response: %w", err)
		}
	}
	return resp.StatusCode, raw, nil
}

func (a *Adapter) signRequest(req *http.Request, method string, payload any) {
	req.Header.Set("va", a.va)
	req.Header.Set("signature", GenerateSignature(method, a.va, a.apiKey, payload))
	req.Header.Set("timestamp", a.timestamp().Format("20060102150405"))
}

func GenerateSignature(method, va, apiKey string, payload any) string {
	var payloadJSON []byte
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	bodyHash := sha256.Sum256(payloadJSON)
	stringToSign := strings.ToUpper(method) + ":" + va + ":" + hex.EncodeToString(bodyHash[:]) + ":" + apiKey
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(stringToSign))
	return hex.EncodeToString(mac.Sum(nil))
}

func GenerateWebhookSignature(apiKey, timestamp, externalID string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(timestamp + ":" + externalID + ":" + string(body)))
	return hex.EncodeToString(mac.Sum(nil))
}

func formSignaturePayload(values url.Values) map[string]any {
	payload := make(map[string]any, len(values))
	for key, list := range values {
		clean := strings.TrimSuffix(key, "[]")
		if strings.HasSuffix(key, "[]") {
			payload[clean] = append([]string(nil), list...)
			continue
		}
		if len(list) == 1 {
			payload[clean] = list[0]
		} else {
			payload[clean] = list
		}
	}
	return payload
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

type redirectPaymentResponse struct {
	Status  int    `json:"Status"`
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Data    struct {
		SessionID string `json:"SessionID"`
		URL       string `json:"Url"`
	} `json:"Data"`
}
type directPaymentResponse struct {
	Status  int    `json:"Status"`
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Data    struct {
		SessionID     string `json:"SessionId"`
		TransactionID int64  `json:"TransactionId"`
		ReferenceID   string `json:"ReferenceId"`
		Via           string `json:"Via"`
		Channel       string `json:"Channel"`
		PaymentNo     string `json:"PaymentNo"`
		PaymentName   string `json:"PaymentName"`
		Total         any    `json:"Total"`
		Fee           any    `json:"Fee"`
		Expired       string `json:"Expired"`
		QrString      string `json:"QrString"`
		QrImage       string `json:"QrImage"`
		QrTemplate    string `json:"QrTemplate"`
	} `json:"Data"`
}
type transactionResponse struct {
	Status  int    `json:"Status"`
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Data    struct {
		TransactionID  int64  `json:"TransactionId"`
		SessionID      string `json:"SessionId"`
		ReferenceID    string `json:"ReferenceId"`
		Amount         int64  `json:"Amount"`
		Fee            int64  `json:"Fee"`
		Status         int    `json:"Status"`
		StatusDesc     string `json:"StatusDesc"`
		PaidStatus     string `json:"PaidStatus"`
		TypeDesc       string `json:"TypeDesc"`
		PaymentChannel string `json:"PaymentChannel"`
		PaymentCode    string `json:"PaymentCode"`
		BuyerName      string `json:"BuyerName"`
		BuyerPhone     string `json:"BuyerPhone"`
		BuyerEmail     string `json:"BuyerEmail"`
	} `json:"Data"`
}
