package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/daemon"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	"github.com/pendig/rute-bayar/internal/forwardingsvc"
	"github.com/pendig/rute-bayar/internal/providerfactory"
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

		handlers, err := factory.WebhookHandlers(ctx, domain.Environment(environmentValue))
		if err != nil {
			return err
		}

		srv := daemon.NewServer(*addr, store, forwarding.NewService(store), handlers)
		fmt.Fprintf(stdout, "Rute Bayar webhook daemon listening on %s\n", *addr)
		fmt.Fprintf(stdout, "webhook environment: %s\n", environmentValue)
		fmt.Fprintf(stdout, "SQLite database: %s\n", *dbPath)
		return srv.ListenAndServe()
	case "replay":
		return webhookReplayCommand(ctx, stdout, stderr, args[1:])
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

	svc := forwardingsvc.New(store)
	var providerFilter domain.ProviderCode
	if trimmedProvider := strings.TrimSpace(*providerCode); trimmedProvider != "" {
		parsed, err := parseProvider(trimmedProvider)
		if err != nil {
			return err
		}
		providerFilter = parsed
	}

	targets, err := svc.List(ctx, forwardingsvc.ListInput{
		Provider:        providerFilter,
		IncludeDisabled: *includeDisabled,
	})
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(w, "no forwarding targets found")
		return nil
	}
	fmt.Fprintln(w, "ID                                      PROVIDER    NAME               URL                                 ENABLED")
	fmt.Fprintln(w, "-------------------------------------------------------------------------------")
	for _, target := range targets {
		fmt.Fprintf(w, "%-40s %-10s %-17s %-35s %t\n", target.ID, target.Provider, target.Name, target.URL, target.Enabled)
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

	provider, err := parseProvider(*providerCode)
	if err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	svc := forwardingsvc.New(store)
	id, err := svc.Add(ctx, forwardingsvc.AddInput{
		Provider:    provider,
		Name:        *name,
		URL:         *targetURL,
		Enabled:     *enabled,
		Headers:     convertSliceMapToHeaders(headerFlags.values),
		EventFilter: filterFlags.values,
		RetryPolicy: forwarding.RetryPolicy{
			MaxAttempts: maxAttempts.value,
			Timeout:     retryTimeout.value,
			Backoff:     retryBackoff.value,
		},
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

	svc := forwardingsvc.New(store)
	var retryPolicy forwarding.RetryPolicy
	if maxAttempts.set {
		retryPolicy.MaxAttempts = maxAttempts.value
	}
	if retryTimeout.set {
		retryPolicy.Timeout = retryTimeout.value
	}
	if retryBackoff.set {
		retryPolicy.Backoff = retryBackoff.value
	}
	if err := svc.Update(ctx, forwardingsvc.UpdateInput{
		ID:             targetID,
		Name:           *name,
		URL:            *targetURL,
		Enabled:        enabled.value,
		EnabledSet:     enabled.set,
		Headers:        convertSliceMapToHeaders(headerFlags.values),
		HeadersSet:     headerFlags.set,
		EventFilter:    filterFlags.values,
		EventFilterSet: filterFlags.set,
		RetryPolicy:    retryPolicy,
		RetryPolicySet: maxAttempts.set || retryTimeout.set || retryBackoff.set,
	}); err != nil {
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

	svc := forwardingsvc.New(store)
	if err := svc.Remove(ctx, targetID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "forwarding target removed: %s\n", targetID)
	return nil
}

func webhookReplayCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook replay requires --event-id")
	}

	cfg := config.Load()
	fs := flag.NewFlagSet("webhook replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "", "provider code: midtrans or xendit")
	eventID := fs.String("event-id", "", "webhook event id stored in webhook_events")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	trimmedEventID := strings.TrimSpace(*eventID)
	if trimmedEventID == "" {
		return fmt.Errorf("webhook replay requires --event-id")
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	event, err := store.GetWebhookEventByID(ctx, trimmedEventID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(*providerCode) != "" {
		requestedProvider, err := parseProvider(*providerCode)
		if err != nil {
			return err
		}
		if event.ProviderCode != requestedProvider {
			return fmt.Errorf("webhook event %s is for provider %q, not %q", trimmedEventID, event.ProviderCode, requestedProvider)
		}
	}

	headers := http.Header{}
	if len(event.HeadersJSON) > 0 {
		headers, err = parseHeadersJSON(event.HeadersJSON)
		if err != nil {
			return fmt.Errorf("parse stored webhook headers: %w", err)
		}
	}

	forwarder := forwarding.NewService(store)
	inbound := forwarding.InboundWebhook{
		WebhookEventID: trimmedEventID,
		Provider:       event.ProviderCode,
		Headers:        headers,
		Body:           event.PayloadJSON,
	}

	if err := forwarder.Forward(ctx, inbound); err != nil {
		return fmt.Errorf("webhook replay failed: %w", err)
	}

	fmt.Fprintf(stdout, "webhook replayed\nid: %s\nprovider: %s\n", trimmedEventID, event.ProviderCode)
	return nil
}

func parseHeadersJSON(raw json.RawMessage) (http.Header, error) {
	if len(raw) == 0 {
		return http.Header{}, nil
	}

	rawEntries := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return nil, err
	}

	headers := make(http.Header, len(rawEntries))
	for key, rawValue := range rawEntries {
		var multiValues []string
		if err := json.Unmarshal(rawValue, &multiValues); err == nil {
			headers[key] = append([]string(nil), multiValues...)
			continue
		}

		var singleValue string
		if err := json.Unmarshal(rawValue, &singleValue); err == nil {
			headers.Set(key, singleValue)
		}
	}

	return headers, nil
}
