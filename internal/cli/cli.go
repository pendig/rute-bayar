package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/build"
	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/daemon"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/provider/midtrans"
	"github.com/pendig/rute-bayar/internal/provider/xendit"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func Execute(args []string) error {
	return ExecuteWithIO(context.Background(), args, os.Stdout, os.Stderr)
}

func ExecuteWithIO(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printHelp(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return nil
	case "version", "--version":
		fmt.Fprintf(stdout, "%s %s\n", build.Name, build.Version)
		return nil
	case "onboard":
		return onboard(ctx, stdout, stderr, args[1:])
	case "provider":
		return providerCommand(ctx, stdout, args[1:])
	case "pay":
		return payCommand(ctx, stdout, stderr, args[1:])
	case "webhook":
		return webhookCommand(ctx, stdout, stderr, args[1:])
	case "db":
		return dbCommand(ctx, stdout, args[1:])
	case "reconcile":
		fmt.Fprintln(stdout, "reconcile command scaffold is ready; implementation comes next.")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `Rute Bayar

Usage:
  rute-bayar onboard
  rute-bayar onboard xendit --secret-key <key>
  rute-bayar onboard midtrans --server-key <key> --client-key <key> --merchant-id <id>
  rute-bayar provider list
  rute-bayar provider accounts
  rute-bayar provider test
  rute-bayar pay create --provider midtrans --method bank_transfer --bank bca
  rute-bayar pay create --provider xendit --method payment_link --reference rb-0001 --amount 15000
  rute-bayar pay status --provider midtrans --reference rb-0001
  rute-bayar pay refund
	rute-bayar webhook serve --addr :8080
	rute-bayar webhook forward list
	rute-bayar webhook forward add
	rute-bayar webhook forward update
	rute-bayar webhook forward remove
  rute-bayar db migrate
  rute-bayar reconcile
  rute-bayar version`)
}

func onboard(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "Available onboarding providers:")
		fmt.Fprintln(stdout, "  xendit")
		fmt.Fprintln(stdout, "  midtrans")
		return nil
	}

	switch args[0] {
	case "xendit":
		return onboardXendit(ctx, stdout, stderr, args[1:])
	case "midtrans":
		return onboardMidtrans(ctx, stdout, stderr, args[1:])
	default:
		return fmt.Errorf("unknown onboarding provider %q", args[0])
	}
}

func onboardMidtrans(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("onboard midtrans", flag.ContinueOnError)
	fs.SetOutput(stderr)
	merchantID := fs.String("merchant-id", "", "Midtrans merchant ID")
	clientKey := fs.String("client-key", "", "Midtrans client key")
	serverKey := fs.String("server-key", "", "Midtrans server key")
	environment := fs.String("environment", cfg.Environment, "provider environment: sandbox or production")
	displayName := fs.String("name", "Midtrans", "provider account display name")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*merchantID) == "" {
		return fmt.Errorf("midtrans --merchant-id is required")
	}
	if strings.TrimSpace(*clientKey) == "" {
		return fmt.Errorf("midtrans --client-key is required")
	}
	if strings.TrimSpace(*serverKey) == "" {
		return fmt.Errorf("midtrans --server-key is required")
	}
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	credentialJSON, err := json.Marshal(midtransCredential{
		MerchantID: strings.TrimSpace(*merchantID),
		ClientKey:  strings.TrimSpace(*clientKey),
		ServerKey:  strings.TrimSpace(*serverKey),
	})
	if err != nil {
		return fmt.Errorf("marshal midtrans credential: %w", err)
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	accountID, err := store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderMidtrans,
		Environment:    domain.Environment(*environment),
		DisplayName:    *displayName,
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "midtrans account saved: %s\n", accountID)
	fmt.Fprintf(stdout, "environment: %s\n", *environment)
	fmt.Fprintf(stdout, "merchant id: %s\n", strings.TrimSpace(*merchantID))
	fmt.Fprintf(stdout, "client key: %s\n", maskSecret(*clientKey))
	fmt.Fprintf(stdout, "server key: %s\n", maskSecret(*serverKey))
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	return nil
}

func onboardXendit(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("onboard xendit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	secretKey := fs.String("secret-key", "", "Xendit secret API key")
	environment := fs.String("environment", cfg.Environment, "provider environment: sandbox or production")
	displayName := fs.String("name", "Xendit", "provider account display name")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	webhookToken := fs.String("webhook-token", "", "optional Xendit webhook verification token")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*secretKey) == "" {
		return fmt.Errorf("xendit --secret-key is required")
	}
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	credentialJSON, err := json.Marshal(map[string]string{
		"secret_key": strings.TrimSpace(*secretKey),
	})
	if err != nil {
		return fmt.Errorf("marshal xendit credential: %w", err)
	}
	configJSON, err := json.Marshal(map[string]string{
		"webhook_token": strings.TrimSpace(*webhookToken),
	})
	if err != nil {
		return fmt.Errorf("marshal xendit config: %w", err)
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	accountID, err := store.UpsertProviderAccount(ctx, domain.ProviderAccount{
		ProviderCode:   domain.ProviderXendit,
		Environment:    domain.Environment(*environment),
		DisplayName:    *displayName,
		CredentialJSON: credentialJSON,
		ConfigJSON:     configJSON,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "xendit account saved: %s\n", accountID)
	fmt.Fprintf(stdout, "environment: %s\n", *environment)
	fmt.Fprintf(stdout, "secret key: %s\n", maskSecret(*secretKey))
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	return nil
}

func providerCommand(ctx context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provider command requires a subcommand")
	}
	switch args[0] {
	case "list":
		for _, provider := range allProviders() {
			fmt.Fprintln(w, provider)
		}
		return nil
	case "accounts":
		return providerAccounts(ctx, w)
	case "test":
		return providerTest(ctx, w, args[1:])
	default:
		return fmt.Errorf("unknown provider subcommand %q", args[0])
	}
}

func providerTest(ctx context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provider test requires a provider code")
	}

	switch args[0] {
	case "midtrans":
		return providerTestMidtrans(ctx, w, args[1:])
	case "xendit":
		return providerTestXendit(ctx, w, args[1:])
	default:
		return fmt.Errorf("provider test for %q is not implemented yet", args[0])
	}
}

func providerTestMidtrans(ctx context.Context, w io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("provider test midtrans", flag.ContinueOnError)
	fs.SetOutput(w)
	environment := fs.String("environment", cfg.Environment, "provider environment: sandbox or production")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	baseURL := fs.String("base-url", "", "override Midtrans API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	account, err := store.GetProviderAccount(ctx, domain.ProviderMidtrans, domain.Environment(*environment))
	if err != nil {
		return err
	}

	credential, err := midtransCredentialFromJSON(account.CredentialJSON)
	if err != nil {
		return err
	}

	options := []midtrans.Option{midtrans.WithServerKey(credential.ServerKey)}
	if strings.TrimSpace(*baseURL) != "" {
		options = append(options, midtrans.WithBaseURL(*baseURL))
	} else {
		options = append(options, midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(domain.Environment(*environment))))
	}
	adapter := midtrans.New(options...)
	info, err := adapter.TestAuth(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "midtrans auth ok")
	fmt.Fprintf(w, "environment: %s\n", *environment)
	if info.StatusCode != "" {
		fmt.Fprintf(w, "status_code: %s\n", info.StatusCode)
	}
	if info.StatusMessage != "" {
		fmt.Fprintf(w, "status_message: %s\n", info.StatusMessage)
	}
	return nil
}

func providerTestXendit(ctx context.Context, w io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("provider test xendit", flag.ContinueOnError)
	fs.SetOutput(w)
	environment := fs.String("environment", cfg.Environment, "provider environment: sandbox or production")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	baseURL := fs.String("base-url", "", "override Xendit API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	account, err := store.GetProviderAccount(ctx, domain.ProviderXendit, domain.Environment(*environment))
	if err != nil {
		return err
	}

	secretKey, err := secretKeyFromCredential(account.CredentialJSON)
	if err != nil {
		return err
	}

	options := []xendit.Option{xendit.WithSecretKey(secretKey)}
	if strings.TrimSpace(*baseURL) != "" {
		options = append(options, xendit.WithBaseURL(*baseURL))
	}
	adapter := xendit.New(options...)
	info, err := adapter.TestAuth(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "xendit auth ok")
	fmt.Fprintf(w, "environment: %s\n", *environment)
	if info.PermissionWarning != "" {
		fmt.Fprintf(w, "warning: %s\n", info.PermissionWarning)
	}
	if info.Balance != nil {
		fmt.Fprintf(w, "balance: %.0f\n", *info.Balance)
	}
	return nil
}

func providerAccounts(ctx context.Context, w io.Writer) error {
	cfg := config.Load()
	store, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	accounts, err := store.ListProviderAccounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Fprintln(w, "no provider accounts configured yet")
		return nil
	}

	for _, account := range accounts {
		fmt.Fprintf(w, "%s %s %s\n", account.ProviderCode, account.Environment, account.DisplayName)
	}
	return nil
}

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
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

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

	var adapter provider.Adapter
	switch normalizedProvider {
	case "midtrans":
		account, err := store.GetProviderAccount(ctx, domain.ProviderMidtrans, domain.Environment(*environment))
		if err != nil {
			return err
		}
		credential, err := midtransCredentialFromJSON(account.CredentialJSON)
		if err != nil {
			return err
		}

		options := []midtrans.Option{midtrans.WithServerKey(credential.ServerKey)}
		if strings.TrimSpace(*baseURL) != "" {
			options = append(options, midtrans.WithBaseURL(*baseURL))
		} else {
			options = append(options, midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(domain.Environment(*environment))))
		}
		adapter = midtrans.New(options...)
	case "xendit":
		secretKey, err := secretKeyFromCredentialFromStore(store, ctx, domain.ProviderXendit, domain.Environment(*environment))
		if err != nil {
			return err
		}

		if request.Method == "" || strings.EqualFold(request.Method, "bank_transfer") {
			request.Method = "payment_link"
		}
		if !isXenditPayMethodSupported(request.Method) {
			return fmt.Errorf("pay create for xendit supports --method payment_link only")
		}

		options := []xendit.Option{xendit.WithSecretKey(secretKey)}
		if strings.TrimSpace(*baseURL) != "" {
			options = append(options, xendit.WithBaseURL(*baseURL))
		}
		adapter = xendit.New(options...)
	default:
		return fmt.Errorf("pay create for provider %q is not implemented yet", *providerCode)
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
	if err := validateEnvironment(*environment); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	intent, intentFound, err := lookupPaymentIntent(ctx, store, *reference)
	if err != nil {
		return err
	}

	providerRef := strings.TrimSpace(*providerReference)
	if providerRef == "" && intentFound {
		if attempt, err := store.GetLatestPaymentAttemptByIntent(ctx, intent.ID, domain.ProviderCode(*providerCode)); err == nil {
			providerRef = strings.TrimSpace(attempt.ProviderReference)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if providerRef == "" {
		providerRef = strings.TrimSpace(*reference)
	}

	switch *providerCode {
	case "midtrans":
		account, err := store.GetProviderAccount(ctx, domain.ProviderMidtrans, domain.Environment(*environment))
		if err != nil {
			return err
		}
		credential, err := midtransCredentialFromJSON(account.CredentialJSON)
		if err != nil {
			return err
		}

		options := []midtrans.Option{midtrans.WithServerKey(credential.ServerKey)}
		if strings.TrimSpace(*baseURL) != "" {
			options = append(options, midtrans.WithBaseURL(*baseURL))
		} else {
			options = append(options, midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(domain.Environment(*environment))))
		}

		adapter := midtrans.New(options...)
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
		account, err := store.GetProviderAccount(ctx, domain.ProviderXendit, domain.Environment(*environment))
		if err != nil {
			return err
		}
		secretKey, err := secretKeyFromCredential(account.CredentialJSON)
		if err != nil {
			return err
		}

		options := []xendit.Option{xendit.WithSecretKey(secretKey)}
		if strings.TrimSpace(*baseURL) != "" {
			options = append(options, xendit.WithBaseURL(*baseURL))
		}

		adapter := xendit.New(options...)
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
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PaymentIntent{}, false, nil
		}
		return domain.PaymentIntent{}, false, err
	}
	return intent, true, nil
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

func webhookCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook command requires a subcommand")
	}

	switch args[0] {
	case "serve":
		fs := flag.NewFlagSet("webhook serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		cfg := config.Load()
		addr := fs.String("addr", cfg.WebhookAddr, "daemon listen address")
		environment := fs.String("environment", cfg.Environment, "webhook provider credential environment")
		dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := validateEnvironment(*environment); err != nil {
			return err
		}

		store, err := sqlite.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		handlers, err := buildWebhookHandlers(ctx, store, domain.Environment(*environment))
		if err != nil {
			return err
		}

		srv := daemon.NewServer(*addr, store, forwarding.NewService(store), handlers)
		fmt.Fprintf(stdout, "Rute Bayar webhook daemon listening on %s\n", *addr)
		fmt.Fprintf(stdout, "webhook environment: %s\n", *environment)
		fmt.Fprintf(stdout, "SQLite database: %s\n", *dbPath)
		return srv.ListenAndServe()
	case "replay":
		fmt.Fprintln(stdout, "webhook replay scaffold is ready.")
		return nil
	case "forward":
		return webhookForwardCommand(ctx, stdout, stderr, args[1:])
	default:
		return fmt.Errorf("unknown webhook subcommand %q", args[0])
	}
}

func dbCommand(ctx context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("db command requires a subcommand")
	}

	switch args[0] {
	case "migrate":
		cfg := config.Load()
		store, err := sqlite.Open(ctx, cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		fmt.Fprintf(w, "database migrated: %s\n", cfg.DBPath)
		return nil
	default:
		return fmt.Errorf("unknown db subcommand %q", args[0])
	}
}

func webhookForwardCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward command requires a subcommand")
	}

	switch args[0] {
	case "list":
		return webhookForwardList(ctx, stdout, args[1:])
	case "add":
		return webhookForwardAdd(ctx, stdout, stderr, args[1:])
	case "update":
		return webhookForwardUpdate(ctx, stdout, stderr, args[1:])
	case "remove":
		return webhookForwardRemove(ctx, stdout, stderr, args[1:])
	default:
		return fmt.Errorf("unknown webhook forward subcommand %q", strings.Join(args, " "))
	}
}

func webhookForwardList(ctx context.Context, w io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward list", flag.ContinueOnError)
	fs.SetOutput(w)
	providerCode := fs.String("provider", "", "filter by provider code: midtrans or xendit")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	includeDisabled := fs.Bool("all", false, "include disabled targets")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	providers := allProviders()
	if trimmedProvider := strings.TrimSpace(*providerCode); trimmedProvider != "" {
		provider, err := parseProvider(trimmedProvider)
		if err != nil {
			return err
		}
		providers = []domain.ProviderCode{provider}
	}

	itemsPrinted := 0
	for _, provider := range providers {
		targets, err := store.ListForwardingTargets(ctx, provider)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if !*includeDisabled && !target.Enabled {
				continue
			}
			if itemsPrinted == 0 {
				fmt.Fprintln(w, "ID                                      PROVIDER    NAME               URL                                 ENABLED")
				fmt.Fprintln(w, "-------------------------------------------------------------------------------")
			}
			itemsPrinted++
			fmt.Fprintf(w, "%-40s %-10s %-17s %-35s %t\n", target.ID, target.Provider, target.Name, target.URL, target.Enabled)
		}
	}

	if itemsPrinted == 0 {
		fmt.Fprintln(w, "no forwarding targets found")
	}
	return nil
}

func webhookForwardAdd(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "midtrans", "provider code: midtrans or xendit")
	name := fs.String("name", "", "forwarding target name")
	targetURL := fs.String("url", "", "destination webhook URL")
	enabled := fs.Bool("enabled", true, "whether target is enabled")
	maxAttempts := &intFlag{value: forwarding.DefaultRetryPolicy().MaxAttempts}
	retryTimeout := &durationFlag{value: forwarding.DefaultRetryPolicy().Timeout}
	retryBackoff := &durationFlag{value: forwarding.DefaultRetryPolicy().Backoff}
	fs.Var(maxAttempts, "retry-max-attempts", "max retry attempts")
	fs.Var(retryTimeout, "retry-timeout", "retry timeout")
	fs.Var(retryBackoff, "retry-backoff", "retry backoff")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	headerFlags := &stringSliceMapFlag{}
	filterFlags := &stringMapFlag{}
	fs.Var(headerFlags, "header", "repeatable outbound request header in key=value form")
	fs.Var(filterFlags, "event-filter", "repeatable outbound event filter in key=value form")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("webhook forward add --name is required")
	}
	if strings.TrimSpace(*targetURL) == "" {
		return fmt.Errorf("webhook forward add --url is required")
	}
	if maxAttempts.value <= 0 {
		return fmt.Errorf("webhook forward add --retry-max-attempts must be greater than zero")
	}
	if retryTimeout.value <= 0 {
		return fmt.Errorf("webhook forward add --retry-timeout must be greater than zero")
	}
	if retryBackoff.value < 0 {
		return fmt.Errorf("webhook forward add --retry-backoff cannot be negative")
	}

	provider, err := parseProvider(*providerCode)
	if err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.AddForwardingTarget(ctx, forwarding.Target{
		Name:        strings.TrimSpace(*name),
		Provider:    provider,
		URL:         strings.TrimSpace(*targetURL),
		Headers:     convertSliceMapToHeaders(headerFlags.values),
		EventFilter: filterFlags.values,
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: maxAttempts.value,
			Timeout:     retryTimeout.value,
			Backoff:     retryBackoff.value,
		},
		Enabled: *enabled,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "forwarding target added\nid: %s\n", id)
	return nil
}

func webhookForwardUpdate(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward update requires target id")
	}
	targetID := strings.TrimSpace(args[0])
	if targetID == "" {
		return fmt.Errorf("webhook forward update target id is required")
	}

	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "forwarding target name")
	targetURL := fs.String("url", "", "destination webhook URL")
	enabled := &boolFlag{value: true}
	maxAttempts := &intFlag{}
	retryTimeout := &durationFlag{}
	retryBackoff := &durationFlag{}
	fs.Var(maxAttempts, "retry-max-attempts", "max retry attempts")
	fs.Var(retryTimeout, "retry-timeout", "retry timeout")
	fs.Var(retryBackoff, "retry-backoff", "retry backoff")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	headerFlags := &stringSliceMapFlag{}
	filterFlags := &stringMapFlag{}
	fs.Var(enabled, "enabled", "whether target is enabled")
	fs.Var(headerFlags, "header", "repeatable outbound request header in key=value form")
	fs.Var(filterFlags, "event-filter", "repeatable outbound event filter in key=value form")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	target, err := store.GetForwardingTarget(ctx, targetID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(*name) != "" {
		target.Name = strings.TrimSpace(*name)
	}
	if strings.TrimSpace(*targetURL) != "" {
		target.URL = strings.TrimSpace(*targetURL)
	}

	if maxAttempts.set {
		if maxAttempts.value <= 0 {
			return fmt.Errorf("webhook forward update --retry-max-attempts must be greater than zero")
		}
		target.RetryPolicy.MaxAttempts = maxAttempts.value
	}
	if retryTimeout.set {
		if retryTimeout.value <= 0 {
			return fmt.Errorf("webhook forward update --retry-timeout must be greater than zero")
		}
		target.RetryPolicy.Timeout = retryTimeout.value
	}
	if retryBackoff.set {
		if retryBackoff.value < 0 {
			return fmt.Errorf("webhook forward update --retry-backoff cannot be negative")
		}
		target.RetryPolicy.Backoff = retryBackoff.value
	}
	if enabled.set {
		target.Enabled = enabled.value
	}

	if headerFlags.set {
		target.Headers = convertSliceMapToHeaders(headerFlags.values)
	}
	if filterFlags.set {
		target.EventFilter = copyStringMap(filterFlags.values)
	}

	if strings.TrimSpace(target.Name) == "" {
		return fmt.Errorf("webhook forward update cannot set empty name")
	}
	if strings.TrimSpace(target.URL) == "" {
		return fmt.Errorf("webhook forward update cannot set empty target URL")
	}

	defaultPolicy := forwarding.DefaultRetryPolicy()
	if target.RetryPolicy.MaxAttempts <= 0 {
		target.RetryPolicy.MaxAttempts = defaultPolicy.MaxAttempts
	}
	if target.RetryPolicy.Timeout <= 0 {
		target.RetryPolicy.Timeout = defaultPolicy.Timeout
	}
	if target.RetryPolicy.Backoff < 0 {
		target.RetryPolicy.Backoff = defaultPolicy.Backoff
	}

	if err := store.UpdateForwardingTarget(ctx, target); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "forwarding target updated: %s\n", targetID)
	return nil
}

func webhookForwardRemove(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward remove requires target id")
	}
	targetID := strings.TrimSpace(args[0])
	if targetID == "" {
		return fmt.Errorf("webhook forward remove target id is required")
	}

	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.DeleteForwardingTarget(ctx, targetID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "forwarding target removed: %s\n", targetID)
	return nil
}

type boolFlag struct {
	value bool
	set   bool
}

func (f *boolFlag) Set(value string) error {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = parsed
	f.set = true
	return nil
}

func (f *boolFlag) String() string {
	return strconv.FormatBool(f.value)
}

type stringMapFlag struct {
	values map[string]string
	set    bool
}

func (f *stringMapFlag) Set(value string) error {
	key, mappedValue, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("invalid key-value pair %q, expected key=value", value)
	}
	key = strings.TrimSpace(key)
	mappedValue = strings.TrimSpace(mappedValue)
	if key == "" {
		return fmt.Errorf("invalid key-value pair %q, key cannot be empty", value)
	}

	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = mappedValue
	f.set = true
	return nil
}

func (f *stringMapFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	return "key=value"
}

type stringSliceMapFlag struct {
	values map[string][]string
	set    bool
}

func (f *stringSliceMapFlag) Set(value string) error {
	key, mappedValue, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("invalid key-value pair %q, expected key=value", value)
	}
	key = strings.TrimSpace(key)
	mappedValue = strings.TrimSpace(mappedValue)
	if key == "" {
		return fmt.Errorf("invalid key-value pair %q, key cannot be empty", value)
	}

	if f.values == nil {
		f.values = map[string][]string{}
	}
	f.values[key] = append(f.values[key], mappedValue)
	f.set = true
	return nil
}

func (f *stringSliceMapFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	return "key=value"
}

func parseProvider(value string) (domain.ProviderCode, error) {
	provider := strings.ToLower(strings.TrimSpace(value))
	for _, supportedProvider := range allProviders() {
		if provider == string(supportedProvider) {
			return supportedProvider, nil
		}
	}

	valid := make([]string, 0, len(allProviders()))
	for _, supportedProvider := range allProviders() {
		valid = append(valid, string(supportedProvider))
	}
	return "", fmt.Errorf("provider must be one of %q", strings.Join(valid, "\", \""))
}

func convertMapToHeaders(values map[string]string) http.Header {
	headers := http.Header{}
	for key, value := range values {
		headers.Add(key, value)
	}
	return headers
}

func convertSliceMapToHeaders(values map[string][]string) http.Header {
	headers := http.Header{}
	for key, list := range values {
		for _, value := range list {
			headers.Add(key, value)
		}
	}
	return headers
}

func copyStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return map[string][]string{}
	}
	copied := make(map[string][]string, len(values))
	for key, list := range values {
		copiedValues := make([]string, len(list))
		copy(copiedValues, list)
		copied[key] = copiedValues
	}
	return copied
}

type intFlag struct {
	value int
	set   bool
}

func (f *intFlag) Set(value string) error {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = v
	f.set = true
	return nil
}

func (f *intFlag) String() string {
	return strconv.Itoa(f.value)
}

type durationFlag struct {
	value time.Duration
	set   bool
}

func (f *durationFlag) Set(value string) error {
	v, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = v
	f.set = true
	return nil
}

func (f *durationFlag) String() string {
	return f.value.String()
}

func allProviders() []domain.ProviderCode {
	return []domain.ProviderCode{domain.ProviderMidtrans, domain.ProviderXendit}
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func buildWebhookHandlers(ctx context.Context, store *sqlite.Store, environment domain.Environment) (map[domain.ProviderCode]provider.Adapter, error) {
	handlers := make(map[domain.ProviderCode]provider.Adapter)

	midtransAccount, err := store.GetProviderAccount(ctx, domain.ProviderMidtrans, environment)
	if err != nil && !errors.Is(err, sqlite.ErrProviderAccountNotConfigured) {
		return nil, fmt.Errorf("load midtrans webhook account: %w", err)
	}
	if err == nil {
		credential, err := midtransCredentialFromJSON(midtransAccount.CredentialJSON)
		if err != nil {
			return nil, fmt.Errorf("load midtrans webhook credential: %w", err)
		}
		handlers[domain.ProviderMidtrans] = midtrans.New(
			midtrans.WithServerKey(credential.ServerKey),
			midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(environment)),
		)
	}

	xenditAccount, err := store.GetProviderAccount(ctx, domain.ProviderXendit, environment)
	if err != nil && !errors.Is(err, sqlite.ErrProviderAccountNotConfigured) {
		return nil, fmt.Errorf("load xendit webhook account: %w", err)
	}
	if err == nil {
		secretKey, err := secretKeyFromCredential(xenditAccount.CredentialJSON)
		if err != nil {
			return nil, fmt.Errorf("load xendit webhook credential: %w", err)
		}
		options := []xendit.Option{xendit.WithSecretKey(secretKey)}
		token, err := xenditWebhookTokenFromConfig(xenditAccount.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("load xendit webhook config: %w", err)
		}
		if token != "" {
			options = append(options, xendit.WithCallbackToken(token))
		}
		handlers[domain.ProviderXendit] = xendit.New(options...)
	}
	return handlers, nil
}

func xenditWebhookTokenFromConfig(raw json.RawMessage) (string, error) {
	var config struct {
		WebhookToken string `json:"webhook_token"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("read xendit config json: %w", err)
	}
	return strings.TrimSpace(config.WebhookToken), nil
}

func validateEnvironment(value string) error {
	switch domain.Environment(value) {
	case domain.EnvironmentSandbox, domain.EnvironmentProduction:
		return nil
	default:
		return fmt.Errorf("environment must be %q or %q", domain.EnvironmentSandbox, domain.EnvironmentProduction)
	}
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func secretKeyFromCredential(raw []byte) (string, error) {
	var credential struct {
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		return "", fmt.Errorf("read credential json: %w", err)
	}
	secretKey := strings.TrimSpace(credential.SecretKey)
	if secretKey == "" {
		return "", fmt.Errorf("xendit secret key is not configured")
	}
	return secretKey, nil
}

func secretKeyFromCredentialFromStore(store *sqlite.Store, ctx context.Context, providerCode domain.ProviderCode, environment domain.Environment) (string, error) {
	account, err := store.GetProviderAccount(ctx, providerCode, environment)
	if err != nil {
		return "", err
	}
	return secretKeyFromCredential(account.CredentialJSON)
}

func isXenditPayMethodSupported(method string) bool {
	return strings.EqualFold(strings.TrimSpace(method), "payment_link") ||
		strings.EqualFold(strings.TrimSpace(method), "payment-link") ||
		strings.EqualFold(strings.TrimSpace(method), "paymentlink") ||
		strings.EqualFold(strings.TrimSpace(method), "")
}

type midtransCredential struct {
	MerchantID string `json:"merchant_id"`
	ClientKey  string `json:"client_key"`
	ServerKey  string `json:"server_key"`
}

func midtransCredentialFromJSON(raw []byte) (midtransCredential, error) {
	var credential midtransCredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return midtransCredential{}, fmt.Errorf("read midtrans credential json: %w", err)
	}
	credential.MerchantID = strings.TrimSpace(credential.MerchantID)
	credential.ClientKey = strings.TrimSpace(credential.ClientKey)
	credential.ServerKey = strings.TrimSpace(credential.ServerKey)
	if credential.MerchantID == "" {
		return midtransCredential{}, fmt.Errorf("midtrans merchant id is not configured")
	}
	if credential.ClientKey == "" {
		return midtransCredential{}, fmt.Errorf("midtrans client key is not configured")
	}
	if credential.ServerKey == "" {
		return midtransCredential{}, fmt.Errorf("midtrans server key is not configured")
	}
	return credential, nil
}
