package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
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
	environmentValue := strings.TrimSpace(*environment)

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	service := paymentsvc.New(store, nil)
	result, err := service.Create(ctx, paymentsvc.CreateInput{
		Provider:      domain.ProviderCode(strings.TrimSpace(*providerCode)),
		Environment:   domain.Environment(environmentValue),
		BaseURL:       strings.TrimSpace(*baseURL),
		ExternalRef:   strings.TrimSpace(*reference),
		Amount:        *amount,
		Currency:      strings.TrimSpace(*currency),
		Method:        strings.TrimSpace(*method),
		Channel:       strings.TrimSpace(*bank),
		CustomerName:  strings.TrimSpace(*customerName),
		CustomerEmail: strings.TrimSpace(*customerEmail),
		CustomerPhone: strings.TrimSpace(*customerPhone),
	})
	if err != nil {
		return err
	}

	printPaymentCreate(stdout, string(result.ProviderCode), result.Reference, result.Response)
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
	environmentValue := strings.TrimSpace(*environment)

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	service := paymentsvc.New(store, nil)
	result, err := service.Status(ctx, paymentsvc.StatusInput{
		Provider:          domain.ProviderCode(strings.TrimSpace(*providerCode)),
		Environment:       domain.Environment(environmentValue),
		BaseURL:           strings.TrimSpace(*baseURL),
		Reference:         strings.TrimSpace(*reference),
		ProviderReference: strings.TrimSpace(*providerReference),
	})
	if err != nil {
		return err
	}

	printPaymentStatus(stdout, string(result.ProviderCode), result.Reference, result.ProviderReference, result.Response)
	return nil
}
