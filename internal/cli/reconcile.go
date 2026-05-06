package cli

import (
	"context"
	"flag"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func reconcileCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "midtrans", "provider code")
	reference := fs.String("reference", "", "external reference / order id")
	providerReference := fs.String("provider-reference", "", "provider-side reference override")
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
	result, err := service.Reconcile(ctx, paymentsvc.ReconcileInput{
		Provider:          domain.ProviderCode(strings.TrimSpace(*providerCode)),
		Environment:       domain.Environment(environmentValue),
		BaseURL:           strings.TrimSpace(*baseURL),
		Reference:         strings.TrimSpace(*reference),
		ProviderReference: strings.TrimSpace(*providerReference),
	})
	if err != nil {
		return err
	}

	printPaymentReconcile(stdout, string(result.ProviderCode), result)
	return nil
}
