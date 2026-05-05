package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		path = "./rute-bayar.sqlite3"
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := s.db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("run migration %s: %w", name, err)
		}
	}

	return nil
}

func (s *Store) ListEnabledTargets(ctx context.Context, provider domain.ProviderCode) ([]forwarding.Target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			name,
			target_url,
			auth_json,
			retry_policy_json,
			enabled
		FROM webhook_forwarding_targets
		WHERE provider_id = (SELECT id FROM providers WHERE code = ?)
		  AND enabled = 1
		ORDER BY created_at ASC
	`, string(provider))
	if err != nil {
		return nil, fmt.Errorf("query forwarding targets: %w", err)
	}
	defer rows.Close()

	targets := make([]forwarding.Target, 0)
	for rows.Next() {
		var (
			target          forwarding.Target
			authJSON        string
			retryPolicyJSON string
			enabled         int
		)
		if err := rows.Scan(&target.ID, &target.Name, &target.URL, &authJSON, &retryPolicyJSON, &enabled); err != nil {
			return nil, fmt.Errorf("scan forwarding target: %w", err)
		}

		target.Provider = provider
		target.Enabled = enabled == 1
		target.Headers = headersFromJSON(authJSON)
		target.RetryPolicy = retryPolicyFromJSON(retryPolicyJSON)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forwarding targets: %w", err)
	}

	return targets, nil
}

func (s *Store) RecordAttempt(ctx context.Context, attempt forwarding.Attempt) error {
	id := newID("fwd_attempt")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	responseJSON := attempt.ResponseJSON
	if len(responseJSON) == 0 {
		responseJSON = []byte("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_forwarding_attempts (
			id,
			webhook_event_id,
			forwarding_target_id,
			request_json,
			response_json,
			status,
			attempt_no,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, attempt.WebhookEventID, attempt.TargetID, string(attempt.RequestJSON), string(responseJSON), attempt.Status, attempt.AttemptNo, now, now)
	if err != nil {
		return fmt.Errorf("record forwarding attempt: %w", err)
	}
	return nil
}

func (s *Store) RecordWebhookEvent(ctx context.Context, event domain.WebhookEvent) (string, error) {
	id := event.ID
	if id == "" {
		id = newID("webhook")
	}
	receivedAt := event.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	eventType := event.EventType
	if eventType == "" {
		eventType = "unknown"
	}
	processingStatus := event.ProcessingStatus
	if processingStatus == "" {
		processingStatus = "received"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_events (
			id,
			provider_id,
			provider_event_id,
			event_type,
			signature_valid,
			payload_json,
			headers_json,
			received_at,
			processed_at,
			processing_status
		)
		VALUES (
			?,
			(SELECT id FROM providers WHERE code = ?),
			?,
			?,
			?,
			?,
			?,
			?,
			NULL,
			?
		)
	`, id, string(event.ProviderCode), nullable(event.ProviderEventID), eventType, boolInt(event.SignatureValid), string(event.PayloadJSON), string(event.HeadersJSON), receivedAt.Format(time.RFC3339Nano), processingStatus)
	if err != nil {
		return "", fmt.Errorf("record webhook event: %w", err)
	}

	return id, nil
}

func (s *Store) UpsertProviderAccount(ctx context.Context, account domain.ProviderAccount) (string, error) {
	id := account.ID
	if id == "" {
		id = newID("provider_account")
	}
	displayName := account.DisplayName
	if displayName == "" {
		displayName = string(account.ProviderCode) + " " + string(account.Environment)
	}
	credentialJSON := account.CredentialJSON
	if len(credentialJSON) == 0 {
		credentialJSON = []byte("{}")
	}
	configJSON := account.ConfigJSON
	if len(configJSON) == 0 {
		configJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_accounts (
			id,
			provider_id,
			environment,
			display_name,
			credential_json,
			config_json,
			created_at,
			updated_at
		)
		VALUES (
			?,
			(SELECT id FROM providers WHERE code = ?),
			?,
			?,
			?,
			?,
			?,
			?
		)
		ON CONFLICT(provider_id, environment)
		DO UPDATE SET
			display_name = excluded.display_name,
			credential_json = excluded.credential_json,
			config_json = excluded.config_json,
			updated_at = excluded.updated_at
	`, id, string(account.ProviderCode), string(account.Environment), displayName, string(credentialJSON), string(configJSON), now, now)
	if err != nil {
		return "", fmt.Errorf("upsert provider account: %w", err)
	}

	var savedID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT pa.id
		FROM provider_accounts pa
		JOIN providers p ON p.id = pa.provider_id
		WHERE p.code = ? AND pa.environment = ?
	`, string(account.ProviderCode), string(account.Environment)).Scan(&savedID); err != nil {
		return "", fmt.Errorf("load provider account id: %w", err)
	}

	return savedID, nil
}

func (s *Store) ListProviderAccounts(ctx context.Context) ([]domain.ProviderAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			pa.id,
			p.code,
			pa.environment,
			pa.display_name,
			pa.credential_json,
			pa.config_json,
			pa.created_at,
			pa.updated_at
		FROM provider_accounts pa
		JOIN providers p ON p.id = pa.provider_id
		ORDER BY p.code ASC, pa.environment ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query provider accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]domain.ProviderAccount, 0)
	for rows.Next() {
		var (
			account       domain.ProviderAccount
			providerCode  string
			environment   string
			credentialRaw string
			configRaw     string
			createdAtRaw  string
			updatedAtRaw  string
		)
		if err := rows.Scan(&account.ID, &providerCode, &environment, &account.DisplayName, &credentialRaw, &configRaw, &createdAtRaw, &updatedAtRaw); err != nil {
			return nil, fmt.Errorf("scan provider account: %w", err)
		}

		account.ProviderCode = domain.ProviderCode(providerCode)
		account.Environment = domain.Environment(environment)
		account.CredentialJSON = json.RawMessage(credentialRaw)
		account.ConfigJSON = json.RawMessage(configRaw)
		account.CreatedAt = parseTime(createdAtRaw)
		account.UpdatedAt = parseTime(updatedAtRaw)
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider accounts: %w", err)
	}

	return accounts, nil
}

func (s *Store) GetProviderAccount(ctx context.Context, provider domain.ProviderCode, environment domain.Environment) (domain.ProviderAccount, error) {
	var (
		account       domain.ProviderAccount
		credentialRaw string
		configRaw     string
		createdAtRaw  string
		updatedAtRaw  string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			pa.id,
			pa.display_name,
			pa.credential_json,
			pa.config_json,
			pa.created_at,
			pa.updated_at
		FROM provider_accounts pa
		JOIN providers p ON p.id = pa.provider_id
		WHERE p.code = ? AND pa.environment = ?
	`, string(provider), string(environment)).Scan(&account.ID, &account.DisplayName, &credentialRaw, &configRaw, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ProviderAccount{}, fmt.Errorf("provider account %s/%s is not configured", provider, environment)
		}
		return domain.ProviderAccount{}, fmt.Errorf("get provider account: %w", err)
	}

	account.ProviderCode = provider
	account.Environment = environment
	account.CredentialJSON = json.RawMessage(credentialRaw)
	account.ConfigJSON = json.RawMessage(configRaw)
	account.CreatedAt = parseTime(createdAtRaw)
	account.UpdatedAt = parseTime(updatedAtRaw)
	return account, nil
}

func (s *Store) AddForwardingTarget(ctx context.Context, target forwarding.Target) (string, error) {
	id := target.ID
	if id == "" {
		id = newID("fwd_target")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	authJSON := headersToJSON(target.Headers)
	retryPolicyJSON := retryPolicyToJSON(target.RetryPolicy)
	enabled := boolInt(target.Enabled)
	if !target.Enabled {
		enabled = 0
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_forwarding_targets (
			id,
			name,
			provider_id,
			event_filter_json,
			target_url,
			auth_json,
			retry_policy_json,
			enabled,
			created_at,
			updated_at
		)
		VALUES (?, ?, (SELECT id FROM providers WHERE code = ?), '{}', ?, ?, ?, ?, ?, ?)
	`, id, target.Name, string(target.Provider), target.URL, authJSON, retryPolicyJSON, enabled, now, now)
	if err != nil {
		return "", fmt.Errorf("add forwarding target: %w", err)
	}

	return id, nil
}

func headersFromJSON(raw string) http.Header {
	if raw == "" || raw == "{}" {
		return http.Header{}
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return http.Header{}
	}
	headers := http.Header{}
	for key, value := range values {
		headers.Set(key, value)
	}
	return headers
}

func headersToJSON(headers http.Header) string {
	values := make(map[string]string, len(headers))
	for key, headerValues := range headers {
		if len(headerValues) > 0 {
			values[key] = headerValues[0]
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func retryPolicyFromJSON(raw string) forwarding.RetryPolicy {
	if raw == "" || raw == "{}" {
		return forwarding.RetryPolicy{}
	}
	var dto struct {
		MaxAttempts int `json:"max_attempts"`
		TimeoutSec  int `json:"timeout_sec"`
		BackoffSec  int `json:"backoff_sec"`
	}
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return forwarding.RetryPolicy{}
	}
	return forwarding.RetryPolicy{
		MaxAttempts: dto.MaxAttempts,
		Timeout:     time.Duration(dto.TimeoutSec) * time.Second,
		Backoff:     time.Duration(dto.BackoffSec) * time.Second,
	}
}

func retryPolicyToJSON(policy forwarding.RetryPolicy) string {
	dto := struct {
		MaxAttempts int `json:"max_attempts"`
		TimeoutSec  int `json:"timeout_sec"`
		BackoffSec  int `json:"backoff_sec"`
	}{
		MaxAttempts: policy.MaxAttempts,
		TimeoutSec:  int(policy.Timeout / time.Second),
		BackoffSec:  int(policy.Backoff / time.Second),
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
