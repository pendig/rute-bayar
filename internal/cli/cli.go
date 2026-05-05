package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

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
  rute-bayar pay status --provider midtrans --reference rb-0001
  rute-bayar pay refund
  rute-bayar webhook serve --addr :8080
  rute-bayar webhook forward list
  rute-bayar webhook forward add
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
		fmt.Fprintln(w, "midtrans")
		fmt.Fprintln(w, "xendit")
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
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	environment := fs.String("environment", cfg.Environment, "provider environment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *providerCode != "midtrans" {
		return fmt.Errorf("pay create for provider %q is not implemented yet", *providerCode)
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

	rawRequestBody := map[string]any{
		"payment_type": *method,
		"transaction_details": map[string]any{
			"order_id":     *reference,
			"gross_amount": *amount,
		},
		"bank_transfer": map[string]any{
			"bank": *bank,
		},
	}
	if strings.TrimSpace(*customerName) != "" || strings.TrimSpace(*customerEmail) != "" || strings.TrimSpace(*customerPhone) != "" {
		rawRequestBody["customer_details"] = map[string]any{
			"first_name": strings.TrimSpace(*customerName),
			"email":      strings.TrimSpace(*customerEmail),
			"phone":      strings.TrimSpace(*customerPhone),
		}
	}
	rawRequest, err := json.Marshal(rawRequestBody)
	if err != nil {
		return fmt.Errorf("marshal pay create request: %w", err)
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

	adapter := midtrans.New(
		midtrans.WithServerKey(credential.ServerKey),
		midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(domain.Environment(*environment))),
	)
	intentID, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ExternalRef:  *reference,
		ProviderCode: domain.ProviderMidtrans,
		Amount:       *amount,
		Currency:     *currency,
		Status:       domain.PaymentStatusPending,
	})
	if err != nil {
		return err
	}

	response, err := adapter.CreatePayment(ctx, provider.CreatePaymentRequest{
		ExternalRef:   *reference,
		Amount:        *amount,
		Currency:      *currency,
		Method:        *method,
		Channel:       *bank,
		CustomerName:  *customerName,
		CustomerEmail: *customerEmail,
		CustomerPhone: *customerPhone,
	})
	if err != nil {
		_, _ = store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
			PaymentIntentID: intentID,
			ProviderCode:    domain.ProviderMidtrans,
			RequestJSON:     rawRequest,
			ResponseJSON:    []byte("{}"),
			Status:          domain.PaymentStatusFailed,
		})
		return err
	}

	if _, err := store.RecordPaymentAttempt(ctx, domain.PaymentAttempt{
		PaymentIntentID:   intentID,
		ProviderCode:      domain.ProviderMidtrans,
		RequestJSON:       response.RawRequestJSON,
		ResponseJSON:      response.RawResponseJSON,
		Status:            response.Status,
		ProviderReference: response.ProviderReference,
	}); err != nil {
		return err
	}
	if _, err := store.UpsertPaymentIntent(ctx, domain.PaymentIntent{
		ID:           intentID,
		ExternalRef:  *reference,
		ProviderCode: domain.ProviderMidtrans,
		Amount:       *amount,
		Currency:     *currency,
		Status:       response.Status,
	}); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "payment created")
	fmt.Fprintf(stdout, "provider: %s\n", *providerCode)
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
		return webhookForwardCommand(ctx, stdout, args[1:])
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

func webhookForwardCommand(_ context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward command requires a subcommand")
	}

	switch args[0] {
	case "list":
		fmt.Fprintln(w, "no forwarding targets configured yet")
	case "add":
		fmt.Fprintln(w, "webhook forwarding target add scaffold is ready.")
	case "update":
		fmt.Fprintln(w, "webhook forwarding target update scaffold is ready.")
	case "remove":
		fmt.Fprintln(w, "webhook forwarding target remove scaffold is ready.")
	default:
		return fmt.Errorf("unknown webhook forward subcommand %q", strings.Join(args, " "))
	}
	return nil
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
