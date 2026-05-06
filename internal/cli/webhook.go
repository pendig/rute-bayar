package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/daemon"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/provider/midtrans"
	"github.com/pendig/rute-bayar/internal/provider/xendit"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

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
		parsed, err := parseProvider(trimmedProvider)
		if err != nil {
			return err
		}
		providers = []domain.ProviderCode{parsed}
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
	fs.Var(enabled, "enabled", "whether target is enabled")
	fs.Var(maxAttempts, "retry-max-attempts", "max retry attempts")
	fs.Var(retryTimeout, "retry-timeout", "retry timeout")
	fs.Var(retryBackoff, "retry-backoff", "retry backoff")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	headerFlags := &stringSliceMapFlag{}
	filterFlags := &stringMapFlag{}
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
