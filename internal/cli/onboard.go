package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

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
	midtransEnvironment := strings.TrimSpace(*environment)
	if err := validateEnvironment(midtransEnvironment); err != nil {
		return err
	}

	credentialJSON, err := json.Marshal(struct {
		MerchantID string `json:"merchant_id"`
		ClientKey  string `json:"client_key"`
		ServerKey  string `json:"server_key"`
	}{
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
		Environment:    domain.Environment(midtransEnvironment),
		DisplayName:    *displayName,
		CredentialJSON: credentialJSON,
		ConfigJSON:     []byte(`{}`),
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "midtrans account saved: %s\n", accountID)
	fmt.Fprintf(stdout, "environment: %s\n", midtransEnvironment)
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
	environmentVal := strings.TrimSpace(*environment)
	if err := validateEnvironment(environmentVal); err != nil {
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
		Environment:    domain.Environment(environmentVal),
		DisplayName:    *displayName,
		CredentialJSON: credentialJSON,
		ConfigJSON:     configJSON,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "xendit account saved: %s\n", accountID)
	fmt.Fprintf(stdout, "environment: %s\n", environmentVal)
	fmt.Fprintf(stdout, "secret key: %s\n", maskSecret(*secretKey))
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	return nil
}
