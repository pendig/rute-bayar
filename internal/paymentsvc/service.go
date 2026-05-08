package paymentsvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/providerfactory"
)

type Store interface {
	providerfactory.AccountLoader
	UpsertPaymentIntent(context.Context, domain.PaymentIntent) (string, error)
	RecordPaymentAttempt(context.Context, domain.PaymentAttempt) (string, error)
	RecordPaymentStatusCheck(context.Context, domain.PaymentStatusCheck) (string, error)
	RecordRefund(context.Context, domain.Refund) (string, error)
	GetPaymentIntentByExternalRef(context.Context, string) (domain.PaymentIntent, error)
	GetLatestPaymentAttemptByIntent(context.Context, string, domain.ProviderCode) (domain.PaymentAttempt, error)
}

type Service struct {
	store   Store
	factory *providerfactory.Factory
}

type CreateInput struct {
	Provider      domain.ProviderCode
	Environment   domain.Environment
	BaseURL       string
	ExternalRef   string
	Amount        int64
	Currency      string
	Method        string
	Channel       string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	CardToken     string
}

type CreateResult struct {
	ProviderCode domain.ProviderCode
	Reference    string
	Response     provider.CreatePaymentResponse
}

type StatusInput struct {
	Provider          domain.ProviderCode
	Environment       domain.Environment
	BaseURL           string
	Reference         string
	ProviderReference string
}

type StatusResult struct {
	ProviderCode      domain.ProviderCode
	Reference         string
	ProviderReference string
	Response          provider.PaymentStatusResponse
}

type RefundInput struct {
	Provider          domain.ProviderCode
	Environment       domain.Environment
	BaseURL           string
	Reference         string
	ProviderReference string
	RefundReference   string
	Amount            int64
	Currency          string
	Reason            string
}

type RefundResult struct {
	ProviderCode      domain.ProviderCode
	Reference         string
	ProviderReference string
	RefundReference   string
	Response          provider.RefundResponse
}

type ReconcileInput struct {
	Provider          domain.ProviderCode
	Environment       domain.Environment
	BaseURL           string
	Reference         string
	ProviderReference string
}

type ReconcileResult struct {
	ProviderCode      domain.ProviderCode
	Reference         string
	ProviderReference string
	LocalStatus       domain.PaymentStatus
	ProviderStatus    domain.PaymentStatus
	Matched           bool
	Updated           bool
	Response          provider.PaymentStatusResponse
}

func New(store Store, factory *providerfactory.Factory) *Service {
	if factory == nil {
		factory = providerfactory.New(store)
	}
	return &Service{store: store, factory: factory}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (CreateResult, error) {
	providerCode, err := normalizeProvider(input.Provider)
	if err != nil {
		return CreateResult{}, err
	}
	if err := validateEnvironment(input.Environment); err != nil {
		return CreateResult{}, err
	}
	if strings.TrimSpace(input.ExternalRef) == "" {
		return CreateResult{}, fmt.Errorf("reference is required")
	}
	if input.Amount <= 0 {
		return CreateResult{}, fmt.Errorf("amount must be greater than zero")
	}
	if s == nil || s.store == nil || s.factory == nil {
		return CreateResult{}, fmt.Errorf("payment service is not configured")
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		currency = "IDR"
	}
	request := provider.CreatePaymentRequest{
		ExternalRef:   strings.TrimSpace(input.ExternalRef),
		Amount:        input.Amount,
		Currency:      currency,
		Method:        strings.TrimSpace(input.Method),
		Channel:       strings.TrimSpace(input.Channel),
		CustomerName:  strings.TrimSpace(input.CustomerName),
		CustomerEmail: strings.TrimSpace(input.CustomerEmail),
		CustomerPhone: strings.TrimSpace(input.CustomerPhone),
		CardToken:     strings.TrimSpace(input.CardToken),
	}
	request, err = normalizeCreateRequest(providerCode, request)
	if err != nil {
		return CreateResult{}, err
	}

	adapter, err := s.factory.AdapterForStoredAccount(ctx, providerCode, input.Environment, input.BaseURL)
	if err != nil {
		return CreateResult{}, err
	}

	intentID, err := s.store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  request.ExternalRef,
		ProviderCode: providerCode,
		Amount:       input.Amount,
		Currency:     currency,
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		return CreateResult{}, err
	}

	response, err := adapter.CreatePayment(ctx, request)
	requestJSON := response.RawRequestJSON
	if len(requestJSON) == 0 {
		if marshaledRequest, marshalErr := json.Marshal(request); marshalErr == nil {
			requestJSON = marshaledRequest
		}
	}
	if err != nil {
		_, _ = s.store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
			PaymentIntentID: intentID,
			ProviderCode:    providerCode,
			RequestJSON:     requestJSON,
			ResponseJSON:    response.RawResponseJSON,
			Status:          domain.PaymentStatusFailed,
		})
		return CreateResult{ProviderCode: providerCode, Reference: request.ExternalRef, Response: response}, err
	}

	if _, err := s.store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      providerCode,
		RequestJSON:       requestJSON,
		ResponseJSON:      response.RawResponseJSON,
		Status:            response.Status,
		ProviderReference: response.ProviderReference,
	}); err != nil {
		return CreateResult{ProviderCode: providerCode, Reference: request.ExternalRef, Response: response}, err
	}
	if _, err := s.store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ID:           intentID,
		ExternalRef:  request.ExternalRef,
		ProviderCode: providerCode,
		Amount:       input.Amount,
		Currency:     request.Currency,
		Status:       response.Status,
	}); err != nil {
		return CreateResult{ProviderCode: providerCode, Reference: request.ExternalRef, Response: response}, err
	}

	return CreateResult{ProviderCode: providerCode, Reference: request.ExternalRef, Response: response}, nil
}

func (s *Service) Status(ctx context.Context, input StatusInput) (StatusResult, error) {
	providerCode, err := normalizeProvider(input.Provider)
	if err != nil {
		return StatusResult{}, err
	}
	if err := validateEnvironment(input.Environment); err != nil {
		return StatusResult{}, err
	}
	if strings.TrimSpace(input.Reference) == "" {
		return StatusResult{}, fmt.Errorf("reference is required")
	}
	if s == nil || s.store == nil || s.factory == nil {
		return StatusResult{}, fmt.Errorf("payment service is not configured")
	}

	intent, intentFound, err := s.lookupPaymentIntent(ctx, strings.TrimSpace(input.Reference))
	if err != nil {
		return StatusResult{}, err
	}

	providerRef, err := s.resolveProviderReference(ctx, providerCode, input.ProviderReference, input.Reference, intent.ID, intentFound)
	if err != nil {
		return StatusResult{}, err
	}

	adapter, err := s.factory.AdapterForStoredAccount(ctx, providerCode, input.Environment, input.BaseURL)
	if err != nil {
		return StatusResult{}, err
	}

	statusResponse, err := adapter.GetPaymentStatus(ctx, providerRef)
	result := StatusResult{ProviderCode: providerCode, Reference: referenceValueForStatus(intentFound, intent, input.Reference), ProviderReference: providerRef, Response: statusResponse}
	if err != nil {
		if intentFound {
			_, _ = s.store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
				PaymentIntentID:   intent.ID,
				ProviderCode:      providerCode,
				RequestJSON:       statusResponse.RawRequestJSON,
				ResponseJSON:      statusResponse.RawResponseJSON,
				Status:            statusResponse.Status,
				ProviderReference: providerRef,
			})
		}
		return result, err
	}

	if intentFound {
		if nextStatus, shouldUpdate := statusUpdateForProviderInquiry(intent.Status, statusResponse.Status); shouldUpdate {
			if _, err := s.store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
				ID:           intent.ID,
				ExternalRef:  intent.ExternalRef,
				ProviderCode: intent.ProviderCode,
				Amount:       intent.Amount,
				Currency:     intent.Currency,
				Status:       nextStatus,
				MetadataJSON: intent.MetadataJSON,
			}); err != nil {
				return result, err
			}
		}
		if _, err := s.store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
			PaymentIntentID:   intent.ID,
			ProviderCode:      providerCode,
			RequestJSON:       statusResponse.RawRequestJSON,
			ResponseJSON:      statusResponse.RawResponseJSON,
			Status:            statusResponse.Status,
			ProviderReference: providerRef,
		}); err != nil {
			return result, err
		}
	}

	return result, nil
}

func (s *Service) Refund(ctx context.Context, input RefundInput) (RefundResult, error) {
	providerCode, err := normalizeProvider(input.Provider)
	if err != nil {
		return RefundResult{}, err
	}
	if err := validateEnvironment(input.Environment); err != nil {
		return RefundResult{}, err
	}
	reference := strings.TrimSpace(input.Reference)
	if reference == "" {
		return RefundResult{}, fmt.Errorf("reference is required")
	}
	if input.Amount < 0 {
		return RefundResult{}, fmt.Errorf("amount cannot be negative")
	}
	if s == nil || s.store == nil || s.factory == nil {
		return RefundResult{}, fmt.Errorf("payment service is not configured")
	}

	intent, intentFound, err := s.lookupPaymentIntent(ctx, reference)
	if err != nil {
		return RefundResult{}, err
	}
	if !intentFound {
		return RefundResult{}, fmt.Errorf("payment intent %s is not configured", reference)
	}

	providerRef, err := s.resolveProviderReference(ctx, providerCode, input.ProviderReference, intent.ExternalRef, intent.ID, true)
	if err != nil {
		return RefundResult{}, err
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		currency = strings.TrimSpace(intent.Currency)
	}
	if currency == "" {
		currency = "IDR"
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "requested by customer"
	}
	refundReference := strings.TrimSpace(input.RefundReference)
	if refundReference == "" {
		refundReference = defaultRefundReference(intent.ExternalRef, input.Amount)
	}

	adapter, err := s.factory.AdapterForStoredAccount(ctx, providerCode, input.Environment, input.BaseURL)
	if err != nil {
		return RefundResult{}, err
	}

	request := provider.RefundRequest{
		ProviderReference: providerRef,
		ReferenceID:       refundReference,
		Amount:            input.Amount,
		Currency:          currency,
		Reason:            reason,
	}
	response, err := adapter.RefundPayment(ctx, request)
	requestJSON := response.RawRequestJSON
	if len(requestJSON) == 0 {
		if marshaledRequest, marshalErr := json.Marshal(request); marshalErr == nil {
			requestJSON = marshaledRequest
		}
	}
	storedProviderReference := strings.TrimSpace(response.ProviderReference)
	if storedProviderReference == "" {
		storedProviderReference = providerRef
	}
	response.ProviderReference = storedProviderReference

	recordStatus := response.Status
	if err != nil {
		recordStatus = domain.PaymentStatusFailed
	} else if recordStatus == "" {
		recordStatus = domain.PaymentStatusPending
	}
	amountToRecord := input.Amount
	if amountToRecord == 0 {
		amountToRecord = intent.Amount
	}
	if _, recordErr := s.store.RecordRefund(ctx, domain.Refund{
		PaymentIntentID:   intent.ID,
		ProviderCode:      providerCode,
		Amount:            amountToRecord,
		Status:            recordStatus,
		RequestJSON:       requestJSON,
		ResponseJSON:      response.RawResponseJSON,
		ProviderReference: storedProviderReference,
	}); recordErr != nil {
		return RefundResult{
			ProviderCode:      providerCode,
			Reference:         intent.ExternalRef,
			ProviderReference: storedProviderReference,
			RefundReference:   refundReference,
			Response:          response,
		}, recordErr
	}

	if err != nil {
		return RefundResult{
			ProviderCode:      providerCode,
			Reference:         intent.ExternalRef,
			ProviderReference: storedProviderReference,
			RefundReference:   refundReference,
			Response:          response,
		}, err
	}

	if response.Status == domain.PaymentStatusRefunded || response.Status == domain.PaymentStatusPartialRefunded {
		if _, err := s.store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
			ID:           intent.ID,
			ExternalRef:  intent.ExternalRef,
			ProviderCode: intent.ProviderCode,
			Amount:       intent.Amount,
			Currency:     intent.Currency,
			Status:       response.Status,
			MetadataJSON: intent.MetadataJSON,
		}); err != nil {
			return RefundResult{
				ProviderCode:      providerCode,
				Reference:         intent.ExternalRef,
				ProviderReference: storedProviderReference,
				RefundReference:   refundReference,
				Response:          response,
			}, err
		}
	}

	return RefundResult{
		ProviderCode:      providerCode,
		Reference:         intent.ExternalRef,
		ProviderReference: storedProviderReference,
		RefundReference:   refundReference,
		Response:          response,
	}, nil
}

func (s *Service) Reconcile(ctx context.Context, input ReconcileInput) (ReconcileResult, error) {
	providerCode, err := normalizeProvider(input.Provider)
	if err != nil {
		return ReconcileResult{}, err
	}
	if err := validateEnvironment(input.Environment); err != nil {
		return ReconcileResult{}, err
	}
	reference := strings.TrimSpace(input.Reference)
	if reference == "" {
		return ReconcileResult{}, fmt.Errorf("reference is required")
	}
	if s == nil || s.store == nil || s.factory == nil {
		return ReconcileResult{}, fmt.Errorf("payment service is not configured")
	}

	intent, intentFound, err := s.lookupPaymentIntent(ctx, reference)
	if err != nil {
		return ReconcileResult{}, err
	}
	if !intentFound {
		return ReconcileResult{}, fmt.Errorf("payment intent %s is not configured", reference)
	}

	providerRef, err := s.resolveProviderReference(ctx, providerCode, input.ProviderReference, intent.ExternalRef, intent.ID, true)
	if err != nil {
		return ReconcileResult{}, err
	}

	adapter, err := s.factory.AdapterForStoredAccount(ctx, providerCode, input.Environment, input.BaseURL)
	if err != nil {
		return ReconcileResult{}, err
	}

	statusResponse, err := adapter.GetPaymentStatus(ctx, providerRef)
	result := ReconcileResult{
		ProviderCode:      providerCode,
		Reference:         intent.ExternalRef,
		ProviderReference: providerRef,
		LocalStatus:       intent.Status,
		ProviderStatus:    statusResponse.Status,
		Matched:           intent.Status == statusResponse.Status,
		Response:          statusResponse,
	}
	if err != nil {
		if _, recordErr := s.store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
			PaymentIntentID:   intent.ID,
			ProviderCode:      providerCode,
			RequestJSON:       statusResponse.RawRequestJSON,
			ResponseJSON:      statusResponse.RawResponseJSON,
			Status:            statusResponse.Status,
			ProviderReference: providerRef,
		}); recordErr != nil {
			return result, recordErr
		}
		return result, err
	}

	nextStatus, shouldUpdate := statusUpdateForProviderInquiry(intent.Status, statusResponse.Status)
	if !result.Matched && shouldUpdate {
		if _, err := s.store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
			ID:           intent.ID,
			ExternalRef:  intent.ExternalRef,
			ProviderCode: intent.ProviderCode,
			Amount:       intent.Amount,
			Currency:     intent.Currency,
			Status:       nextStatus,
			MetadataJSON: intent.MetadataJSON,
		}); err != nil {
			return result, err
		}
		result.Updated = true
	}

	if _, err := s.store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
		PaymentIntentID:   intent.ID,
		ProviderCode:      providerCode,
		RequestJSON:       statusResponse.RawRequestJSON,
		ResponseJSON:      statusResponse.RawResponseJSON,
		Status:            statusResponse.Status,
		ProviderReference: providerRef,
	}); err != nil {
		return result, err
	}

	return result, nil
}

func (s *Service) lookupPaymentIntent(ctx context.Context, reference string) (domain.PaymentIntent, bool, error) {
	intent, err := s.store.GetPaymentIntentByExternalRef(ctx, reference)
	if err != nil {
		if isNoRowsErr(err) {
			return domain.PaymentIntent{}, false, nil
		}
		return domain.PaymentIntent{}, false, err
	}
	return intent, true, nil
}

func normalizeProvider(value domain.ProviderCode) (domain.ProviderCode, error) {
	switch domain.ProviderCode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case domain.ProviderMidtrans:
		return domain.ProviderMidtrans, nil
	case domain.ProviderXendit:
		return domain.ProviderXendit, nil
	default:
		return "", fmt.Errorf("provider must be one of %q or %q", domain.ProviderMidtrans, domain.ProviderXendit)
	}
}

func validateEnvironment(value domain.Environment) error {
	switch value {
	case domain.EnvironmentSandbox, domain.EnvironmentProduction:
		return nil
	default:
		return fmt.Errorf("environment must be %q or %q", domain.EnvironmentSandbox, domain.EnvironmentProduction)
	}
}

func normalizeCreateRequest(providerCode domain.ProviderCode, request provider.CreatePaymentRequest) (provider.CreatePaymentRequest, error) {
	if providerCode != domain.ProviderXendit {
		return request, nil
	}

	if request.Method == "" || strings.EqualFold(request.Method, "bank_transfer") {
		request.Method = "payment_link"
	}
	if !isXenditPayMethodSupported(request.Method) {
		return provider.CreatePaymentRequest{}, fmt.Errorf("xendit supports payment_link method only")
	}
	request.Method = "payment_link"
	return request, nil
}

func referenceValueForStatus(found bool, intent domain.PaymentIntent, fallback string) string {
	if found && strings.TrimSpace(intent.ExternalRef) != "" {
		return intent.ExternalRef
	}
	return fallback
}

func statusUpdateForProviderInquiry(current, incoming domain.PaymentStatus) (domain.PaymentStatus, bool) {
	if incoming == "" || shouldPreserveRefundStatus(current, incoming) {
		return current, false
	}
	return incoming, current != incoming
}

func shouldPreserveRefundStatus(current, incoming domain.PaymentStatus) bool {
	if !isRefundStatus(current) {
		return false
	}
	return !isRefundStatus(incoming)
}

func isRefundStatus(status domain.PaymentStatus) bool {
	return status == domain.PaymentStatusRefunded || status == domain.PaymentStatusPartialRefunded
}

func (s *Service) resolveProviderReference(ctx context.Context, providerCode domain.ProviderCode, explicit string, fallback string, intentID string, lookupLatest bool) (string, error) {
	providerRef := strings.TrimSpace(explicit)
	if providerRef == "" && lookupLatest && strings.TrimSpace(intentID) != "" {
		if attempt, err := s.store.GetLatestPaymentAttemptByIntent(ctx, intentID, providerCode); err == nil {
			providerRef = strings.TrimSpace(attempt.ProviderReference)
		} else if !isNoRowsErr(err) {
			return "", err
		}
	}
	if providerRef == "" {
		providerRef = strings.TrimSpace(fallback)
	}
	return providerRef, nil
}

func defaultRefundReference(reference string, amount int64) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "refund"
	}
	if amount > 0 {
		return fmt.Sprintf("%s-refund-%d", reference, amount)
	}
	return reference + "-refund"
}

func isNoRowsErr(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func isXenditPayMethodSupported(method string) bool {
	return strings.EqualFold(strings.TrimSpace(method), "payment_link") ||
		strings.EqualFold(strings.TrimSpace(method), "payment-link") ||
		strings.EqualFold(strings.TrimSpace(method), "paymentlink") ||
		strings.EqualFold(strings.TrimSpace(method), "")
}
