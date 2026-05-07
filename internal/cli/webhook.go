package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	case "attempts":
		return webhookForwardAttemptsCommand(ctx, stdout, stderr, args[1:])
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

func webhookForwardAttemptsCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward attempts requires a subcommand")
	}

	switch args[0] {
	case "list":
		return webhookForwardAttemptsList(ctx, stdout, stderr, args[1:])
	case "show":
		return webhookForwardAttemptsShow(ctx, stdout, stderr, args[1:])
	case "retry":
		return webhookForwardAttemptsRetry(ctx, stdout, stderr, args[1:])
	default:
		return fmt.Errorf("unknown webhook forward attempts subcommand %q", strings.Join(args, " "))
	}
}

func webhookForwardAttemptsList(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward attempts list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerCode := fs.String("provider", "", "filter by provider code: midtrans or xendit")
	targetID := fs.String("target-id", "", "filter by forwarding target id")
	eventID := fs.String("event-id", "", "filter by webhook event id")
	status := fs.String("status", "", "filter by forwarding status: success or failed")
	limit := fs.Int("limit", 20, "maximum attempts to list")
	jsonOutput := fs.Bool("json", false, "print attempts as JSON")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	providerFilter, err := parseOptionalProvider(*providerCode)
	if err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	attempts, err := store.ListForwardingAttempts(ctx, forwarding.AttemptFilter{
		Provider:       providerFilter,
		TargetID:       *targetID,
		WebhookEventID: *eventID,
		Status:         *status,
		Limit:          *limit,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printForwardingAttemptsJSON(stdout, attempts, true)
	}
	if len(attempts) == 0 {
		fmt.Fprintln(stdout, "no forwarding attempts found")
		return nil
	}

	fmt.Fprintln(stdout, "ID                                      PROVIDER    TARGET             STATUS   TRY EVENT_ID                              CREATED_AT")
	fmt.Fprintln(stdout, "-----------------------------------------------------------------------------------------------------------------------------")
	for _, attempt := range attempts {
		fmt.Fprintf(stdout, "%-40s %-10s %-18s %-8s %-3d %-37s %s\n",
			attempt.ID,
			attempt.Provider,
			truncate(attempt.TargetName, 18),
			attempt.Status,
			attempt.AttemptNo,
			truncate(attempt.WebhookEventID, 37),
			attempt.CreatedAt.Format(time.RFC3339),
		)
	}
	return nil
}

func webhookForwardAttemptsShow(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward attempts show requires attempt id")
	}
	attemptID := strings.TrimSpace(args[0])
	if attemptID == "" {
		return fmt.Errorf("webhook forward attempts show attempt id is required")
	}

	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward attempts show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "print attempt as JSON")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	attempt, err := store.GetForwardingAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printForwardingAttemptsJSON(stdout, []forwarding.AttemptRecord{attempt}, true)
	}

	fmt.Fprintln(stdout, "forwarding attempt")
	fmt.Fprintf(stdout, "id: %s\n", attempt.ID)
	fmt.Fprintf(stdout, "provider: %s\n", attempt.Provider)
	fmt.Fprintf(stdout, "target_id: %s\n", attempt.TargetID)
	fmt.Fprintf(stdout, "target_name: %s\n", attempt.TargetName)
	fmt.Fprintf(stdout, "target_url: %s\n", attempt.TargetURL)
	fmt.Fprintf(stdout, "webhook_event_id: %s\n", attempt.WebhookEventID)
	fmt.Fprintf(stdout, "status: %s\n", attempt.Status)
	fmt.Fprintf(stdout, "attempt_no: %d\n", attempt.AttemptNo)
	fmt.Fprintf(stdout, "created_at: %s\n", attempt.CreatedAt.Format(time.RFC3339))
	fmt.Fprintln(stdout, "request_json:")
	fmt.Fprintln(stdout, prettyJSON(attempt.RequestJSON))
	fmt.Fprintln(stdout, "response_json:")
	fmt.Fprintln(stdout, prettyJSON(attempt.ResponseJSON))
	return nil
}

func webhookForwardAttemptsRetry(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward attempts retry requires attempt id")
	}
	attemptID := strings.TrimSpace(args[0])
	if attemptID == "" {
		return fmt.Errorf("webhook forward attempts retry attempt id is required")
	}

	cfg := config.Load()
	fs := flag.NewFlagSet("webhook forward attempts retry", flag.ContinueOnError)
	fs.SetOutput(stderr)
	forceDisabled := fs.Bool("force-disabled", false, "allow retry to a disabled forwarding target")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	attempt, err := store.GetForwardingAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	target, err := store.GetForwardingTarget(ctx, attempt.TargetID)
	if err != nil {
		return fmt.Errorf("get forwarding target %q: %w", attempt.TargetID, err)
	}
	if !target.Enabled && !*forceDisabled {
		return fmt.Errorf("forwarding target %q is disabled; use --force-disabled to retry anyway", target.ID)
	}

	event, err := store.GetWebhookEventByID(ctx, attempt.WebhookEventID)
	if err != nil {
		return fmt.Errorf("get webhook event %q: %w", attempt.WebhookEventID, err)
	}
	if event.ProviderCode != target.Provider {
		return fmt.Errorf("webhook event provider %q does not match target provider %q", event.ProviderCode, target.Provider)
	}

	headers, err := parseHeadersJSON(event.HeadersJSON)
	if err != nil {
		return fmt.Errorf("parse stored webhook headers: %w", err)
	}
	inbound := forwarding.InboundWebhook{
		WebhookEventID: event.ID,
		Provider:       event.ProviderCode,
		Headers:        headers,
		Body:           event.PayloadJSON,
	}
	if err := forwarding.NewService(store).ForwardToTarget(ctx, target, inbound); err != nil {
		return fmt.Errorf("retry forwarding attempt %q: %w", attempt.ID, err)
	}

	fmt.Fprintf(stdout, "forwarding attempt retried\nsource_attempt_id: %s\ntarget_id: %s\nwebhook_event_id: %s\n", attempt.ID, target.ID, event.ID)
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
		return fmt.Errorf("get webhook event %q: %w", trimmedEventID, err)
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

func parseOptionalProvider(value string) (domain.ProviderCode, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	return parseProvider(trimmed)
}

type forwardingAttemptOutput struct {
	ID             string          `json:"id"`
	Provider       string          `json:"provider"`
	TargetID       string          `json:"target_id"`
	TargetName     string          `json:"target_name"`
	TargetURL      string          `json:"target_url"`
	WebhookEventID string          `json:"webhook_event_id"`
	Status         string          `json:"status"`
	AttemptNo      int             `json:"attempt_no"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	RequestJSON    json.RawMessage `json:"request_json,omitempty"`
	ResponseJSON   json.RawMessage `json:"response_json,omitempty"`
}

func printForwardingAttemptsJSON(w io.Writer, attempts []forwarding.AttemptRecord, includeRaw bool) error {
	items := make([]forwardingAttemptOutput, 0, len(attempts))
	for _, attempt := range attempts {
		item := forwardingAttemptOutput{
			ID:             attempt.ID,
			Provider:       string(attempt.Provider),
			TargetID:       attempt.TargetID,
			TargetName:     attempt.TargetName,
			TargetURL:      attempt.TargetURL,
			WebhookEventID: attempt.WebhookEventID,
			Status:         attempt.Status,
			AttemptNo:      attempt.AttemptNo,
			CreatedAt:      attempt.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      attempt.UpdatedAt.Format(time.RFC3339),
		}
		if includeRaw {
			item.RequestJSON = rawJSONOrString(attempt.RequestJSON)
			item.ResponseJSON = rawJSONOrString(attempt.ResponseJSON)
		}
		items = append(items, item)
	}
	encoded, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(encoded))
	return nil
}

func rawJSONOrString(raw []byte) json.RawMessage {
	if len(raw) == 0 || json.Valid(raw) {
		return json.RawMessage(raw)
	}
	encoded, _ := json.Marshal(string(raw))
	return json.RawMessage(encoded)
}

func prettyJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return string(raw)
	}
	return out.String()
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	return value[:max-1] + "."
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
			for _, value := range multiValues {
				headers.Add(key, value)
			}
			continue
		}

		var singleValue string
		if err := json.Unmarshal(rawValue, &singleValue); err == nil {
			headers.Set(key, singleValue)
		}
	}

	return headers, nil
}
