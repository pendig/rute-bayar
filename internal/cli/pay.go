package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/providerfactory"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func payCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("pay command requires a subcommand")
	}
	switch args[0] {
	case "create":
		return payCreate(ctx, stdout, stderr, args[1:])
	case "status":
		return payStatus(ctx, stdout, stderr, args[1:])
	case "refund":
		fmt.Fprintln(stdout, "pay refund scaffold is ready.")
		return nil
	default:
		return fmt.Errorf("unknown pay subcommand %q", args[0])
	}
}

func payCreate(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("pay create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "midtrans", "provider code")
	method := fs.String("method", "bank_transfer", "payment method")
	bank := fs.String("bank", "bca", "bank code for bank transfer")
	reference := fs.String("reference", "", "external reference / order id")
	amount := fs.Int64("amount", 0, "payment amount")
	currency := fs.String("currency", "IDR", "payment currency")
	customerName := fs.String("customer-name", "", "customer name")
	customerEmail := fs.String("customer-email", "", "customer email")
	customerPhone := fs.String("customer-phone", "", "customer phone")
	baseURL := fs.String("base-url", "", "override provider API base URL")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	environment := fs.String("environment", cfg.Environment, "provider environment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*reference) == "" {
		return fmt.Errorf("pay create --reference is required")
	}
	if *amount <= 0 {
		return fmt.Errorf("pay create --amount must be greater than zero")
	}
	environmentValue := strings.TrimSpace(*environment)
	if err := validateEnvironment(environmentValue); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	factory := providerfactory.New(store)

	request := provider.CreatePaymentRequest{
		ExternalRef:   strings.TrimSpace(*reference),
		Amount:        *amount,
		Currency:      strings.TrimSpace(*currency),
		Method:        strings.TrimSpace(*method),
		Channel:       strings.TrimSpace(*bank),
		CustomerName:  strings.TrimSpace(*customerName),
		CustomerEmail: strings.TrimSpace(*customerEmail),
		CustomerPhone: strings.TrimSpace(*customerPhone),
	}
	normalizedProvider := strings.ToLower(strings.TrimSpace(*providerCode))

	switch normalizedProvider {
	case "xendit":
		if request.Method == "" || strings.EqualFold(request.Method, "bank_transfer") {
			request.Method = "payment_link"
		}
		if !isXenditPayMethodSupported(request.Method) {
			return fmt.Errorf("pay create for xendit supports --method payment_link only")
		}
	case "midtrans":
	default:
		return fmt.Errorf("pay create for provider %q is not implemented yet", *providerCode)
	}

	adapter, err := factory.AdapterForStoredAccount(
		ctx,
		domain.ProviderCode(normalizedProvider),
		domain.Environment(environmentValue),
		*baseURL,
	)
	if err != nil {
		return err
	}

	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  request.ExternalRef,
		ProviderCode: domain.ProviderCode(normalizedProvider),
		Amount:       *amount,
		Currency:     *currency,
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		return err
	}

	response, err := adapter.CreatePayment(ctx, request)
	requestJSON := response.RawRequestJSON
	if len(requestJSON) == 0 {
		marshaledRequest, marshalErr := json.Marshal(request)
		if marshalErr == nil {
			requestJSON = marshaledRequest
		}
	}
	if err != nil {
		_, _ = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
			PaymentIntentID: intentID,
			ProviderCode:    domain.ProviderCode(normalizedProvider),
			RequestJSON:     requestJSON,
			ResponseJSON:    response.RawResponseJSON,
			Status:          domain.PaymentStatusFailed,
		})
		return err
	}

	if _, err := store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderCode(normalizedProvider),
		RequestJSON:       response.RawRequestJSON,
		ResponseJSON:      response.RawResponseJSON,
		Status:            response.Status,
		ProviderReference: response.ProviderReference,
	}); err != nil {
		return err
	}
	if _, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ID:           intentID,
		ExternalRef:  request.ExternalRef,
		ProviderCode: domain.ProviderCode(normalizedProvider),
		Amount:       *amount,
		Currency:     *currency,
		Status:       response.Status,
	}); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "payment created")
	fmt.Fprintf(stdout, "provider: %s\n", normalizedProvider)
	fmt.Fprintf(stdout, "reference: %s\n", *reference)
	fmt.Fprintf(stdout, "status: %s\n", response.Status)
	if response.TransactionID != "" {
		fmt.Fprintf(stdout, "transaction_id: %s\n", response.TransactionID)
	}
	if response.PaymentType != "" {
		fmt.Fprintf(stdout, "payment_type: %s\n", response.PaymentType)
	}
	if response.VANumber != "" {
		fmt.Fprintf(stdout, "va_number: %s\n", response.VANumber)
	}
	if response.ExpiryTime != "" {
		fmt.Fprintf(stdout, "expiry_time: %s\n", response.ExpiryTime)
	}
	if response.RedirectURL != "" {
		fmt.Fprintf(stdout, "redirect_url: %s\n", response.RedirectURL)
	}
	return nil
}

func payStatus(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("pay status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "midtrans", "provider code")
	reference := fs.String("reference", "", "external reference / order id")
	providerReference := fs.String("provider-reference", "", "provider-side reference override")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	environment := fs.String("environment", cfg.Environment, "provider environment")
	baseURL := fs.String("base-url", "", "override provider API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*reference) == "" {
		return fmt.Errorf("pay status --reference is required")
	}
	environmentValue := strings.TrimSpace(*environment)
	if err := validateEnvironment(environmentValue); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	factory := providerfactory.New(store)

	intent, intentFound, err := lookupPaymentIntent(ctx, store, *reference)
	if err != nil {
		return err
	}

	providerRef := strings.TrimSpace(*providerReference)
	if providerRef == "" && intentFound {
		if attempt, err := store.GetLatestPaymentAttemptByIntent(ctx, intent.ID, domain.ProviderCode(*providerCode)); err == nil {
			providerRef = strings.TrimSpace(attempt.ProviderReference)
		} else if !isNoRowsErr(err) {
			return err
		}
	}
	if providerRef == "" {
		providerRef = strings.TrimSpace(*reference)
	}

	switch strings.ToLower(strings.TrimSpace(*providerCode)) {
	case "midtrans":
		adapter, err := factory.AdapterForStoredAccount(
			ctx,
			domain.ProviderMidtrans,
			domain.Environment(environmentValue),
			*baseURL,
		)
		if err != nil {
			return err
		}
		statusResponse, err := adapter.GetPaymentStatus(ctx, providerRef)
		if err != nil {
			if intentFound {
				_, _ = store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
					PaymentIntentID:   intent.ID,
					ProviderCode:      domain.ProviderMidtrans,
					RequestJSON:       statusResponse.RawRequestJSON,
					ResponseJSON:      statusResponse.RawResponseJSON,
					Status:            statusResponse.Status,
					ProviderReference: providerRef,
				})
			}
			return err
		}

		if intentFound {
			if _, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
				ID:           intent.ID,
				ExternalRef:  intent.ExternalRef,
				ProviderCode: intent.ProviderCode,
				Amount:       intent.Amount,
				Currency:     intent.Currency,
				Status:       statusResponse.Status,
				MetadataJSON: intent.MetadataJSON,
			}); err != nil {
				return err
			}
			if _, err := store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
				PaymentIntentID:   intent.ID,
				ProviderCode:      domain.ProviderMidtrans,
				RequestJSON:       statusResponse.RawRequestJSON,
				ResponseJSON:      statusResponse.RawResponseJSON,
				Status:            statusResponse.Status,
				ProviderReference: providerRef,
			}); err != nil {
				return err
			}
		}

		printPaymentStatus(stdout, "midtrans", referenceValueForStatus(intentFound, intent, *reference), providerRef, statusResponse)
		return nil
	case "xendit":
		adapter, err := factory.AdapterForStoredAccount(
			ctx,
			domain.ProviderXendit,
			domain.Environment(environmentValue),
			*baseURL,
		)
		if err != nil {
			return err
		}
		statusResponse, err := adapter.GetPaymentStatus(ctx, providerRef)
		if err != nil {
			if intentFound {
				_, _ = store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
					PaymentIntentID:   intent.ID,
					ProviderCode:      domain.ProviderXendit,
					RequestJSON:       statusResponse.RawRequestJSON,
					ResponseJSON:      statusResponse.RawResponseJSON,
					Status:            statusResponse.Status,
					ProviderReference: providerRef,
				})
			}
			return err
		}

		if intentFound {
			if _, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
				ID:           intent.ID,
				ExternalRef:  intent.ExternalRef,
				ProviderCode: intent.ProviderCode,
				Amount:       intent.Amount,
				Currency:     intent.Currency,
				Status:       statusResponse.Status,
				MetadataJSON: intent.MetadataJSON,
			}); err != nil {
				return err
			}
			if _, err := store.RecordPaymentStatusCheck(ctx, domain.PaymentStatusCheck{
				PaymentIntentID:   intent.ID,
				ProviderCode:      domain.ProviderXendit,
				RequestJSON:       statusResponse.RawRequestJSON,
				ResponseJSON:      statusResponse.RawResponseJSON,
				Status:            statusResponse.Status,
				ProviderReference: providerRef,
			}); err != nil {
				return err
			}
		}

		printPaymentStatus(stdout, "xendit", referenceValueForStatus(intentFound, intent, *reference), providerRef, statusResponse)
		return nil
	default:
		return fmt.Errorf("pay status for provider %q is not implemented yet", *providerCode)
	}
}

func lookupPaymentIntent(ctx context.Context, store *sqlite.Store, reference string) (domain.PaymentIntent, bool, error) {
	intent, err := store.GetPaymentIntentByExternalRef(ctx, reference)
	if err != nil {
		if isNoRowsErr(err) {
			return domain.PaymentIntent{}, false, nil
		}
		return domain.PaymentIntent{}, false, err
	}
	return intent, true, nil
}

func isNoRowsErr(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func referenceValueForStatus(found bool, intent domain.PaymentIntent, fallback string) string {
	if found && strings.TrimSpace(intent.ExternalRef) != "" {
		return intent.ExternalRef
	}
	return fallback
}

func printPaymentStatus(w io.Writer, providerCode, reference, providerReference string, response provider.PaymentStatusResponse) {
	fmt.Fprintln(w, "payment status")
	fmt.Fprintf(w, "provider: %s\n", providerCode)
	fmt.Fprintf(w, "reference: %s\n", reference)
	if providerReference != "" {
		fmt.Fprintf(w, "provider_reference: %s\n", providerReference)
	}
	fmt.Fprintf(w, "status: %s\n", response.Status)
	if response.StatusCode != "" {
		fmt.Fprintf(w, "status_code: %s\n", response.StatusCode)
	}
	if response.StatusMessage != "" {
		fmt.Fprintf(w, "status_message: %s\n", response.StatusMessage)
	}
	if response.TransactionID != "" {
		fmt.Fprintf(w, "transaction_id: %s\n", response.TransactionID)
	}
	if response.OrderID != "" {
		fmt.Fprintf(w, "order_id: %s\n", response.OrderID)
	}
	if response.PaymentType != "" {
		fmt.Fprintf(w, "payment_type: %s\n", response.PaymentType)
	}
	if response.TransactionStatus != "" {
		fmt.Fprintf(w, "transaction_status: %s\n", response.TransactionStatus)
	}
	if response.FraudStatus != "" {
		fmt.Fprintf(w, "fraud_status: %s\n", response.FraudStatus)
	}
	if response.VANumber != "" {
		fmt.Fprintf(w, "va_number: %s\n", response.VANumber)
	}
	if response.ExpiryTime != "" {
		fmt.Fprintf(w, "expiry_time: %s\n", response.ExpiryTime)
	}
	if response.RedirectURL != "" {
		fmt.Fprintf(w, "redirect_url: %s\n", response.RedirectURL)
	}
}
