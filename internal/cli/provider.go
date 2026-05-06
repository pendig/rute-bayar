package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider/midtrans"
	"github.com/pendig/rute-bayar/internal/provider/xendit"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

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
