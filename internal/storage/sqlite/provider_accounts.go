package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

var ErrProviderAccountNotConfigured = errors.New("provider account is not configured")

func (s *Store) UpsertProviderAccount(ctx context.Context, account domain.ProviderAccount) (string, error) {
	id := account.ID
	if id == "" {
		id = newID("provider_account")
	}
	displayName := strings.TrimSpace(account.DisplayName)
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
		account, err := scanProviderAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider accounts: %w", err)
	}
	return accounts, nil
}

func (s *Store) ListProviderAccountsByFilter(ctx context.Context, providerFilter domain.ProviderCode, environmentFilter domain.Environment) ([]domain.ProviderAccount, error) {
	accounts, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]domain.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if providerFilter != "" && account.ProviderCode != providerFilter {
			continue
		}
		if environmentFilter != "" && account.Environment != environmentFilter {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered, nil
}

func (s *Store) GetProviderAccount(ctx context.Context, provider domain.ProviderCode, environment domain.Environment) (domain.ProviderAccount, error) {
	var (
		account       domain.ProviderAccount
		providerCode  string
		environmentID string
		credentialRaw string
		configRaw     string
		createdAtRaw  string
		updatedAtRaw  string
	)
	if err := s.db.QueryRowContext(ctx, `
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
		WHERE p.code = ? AND pa.environment = ?
	`, string(provider), string(environment)).Scan(&account.ID, &providerCode, &environmentID, &account.DisplayName, &credentialRaw, &configRaw, &createdAtRaw, &updatedAtRaw); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProviderAccount{}, fmt.Errorf("%w: %s/%s", ErrProviderAccountNotConfigured, provider, environment)
		}
		return domain.ProviderAccount{}, fmt.Errorf("get provider account: %w", err)
	}

	account.ProviderCode = domain.ProviderCode(providerCode)
	account.Environment = domain.Environment(environmentID)
	account.CredentialJSON = json.RawMessage(credentialRaw)
	account.ConfigJSON = json.RawMessage(configRaw)
	account.CreatedAt = parseTime(createdAtRaw)
	account.UpdatedAt = parseTime(updatedAtRaw)
	return account, nil
}

func (s *Store) GetProviderAccountByID(ctx context.Context, accountID string) (domain.ProviderAccount, error) {
	var (
		account       domain.ProviderAccount
		providerCode  string
		environment   string
		credentialRaw string
		configRaw     string
		createdAtRaw  string
		updatedAtRaw  string
	)
	if err := s.db.QueryRowContext(ctx, `
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
		WHERE pa.id = ?
	`, strings.TrimSpace(accountID)).Scan(&account.ID, &providerCode, &environment, &account.DisplayName, &credentialRaw, &configRaw, &createdAtRaw, &updatedAtRaw); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProviderAccount{}, err
		}
		return domain.ProviderAccount{}, fmt.Errorf("get provider account by id: %w", err)
	}

	account.ProviderCode = domain.ProviderCode(providerCode)
	account.Environment = domain.Environment(strings.TrimSpace(environment))
	account.CredentialJSON = json.RawMessage(credentialRaw)
	account.ConfigJSON = json.RawMessage(configRaw)
	account.CreatedAt = parseTime(createdAtRaw)
	account.UpdatedAt = parseTime(updatedAtRaw)
	return account, nil
}

func (s *Store) UpdateProviderAccountByID(ctx context.Context, accountID string, account domain.ProviderAccount) (string, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return "", fmt.Errorf("provider account id is required")
	}

	current, err := s.GetProviderAccountByID(ctx, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("provider account %q not found", accountID)
		}
		return "", err
	}

	displayName := strings.TrimSpace(account.DisplayName)
	if displayName == "" {
		displayName = current.DisplayName
	}
	credentialJSON := account.CredentialJSON
	if len(credentialJSON) == 0 {
		credentialJSON = current.CredentialJSON
	}
	configJSON := account.ConfigJSON
	if len(configJSON) == 0 {
		configJSON = current.ConfigJSON
	}
	environment := strings.TrimSpace(string(account.Environment))
	if environment == "" {
		environment = string(current.Environment)
	}
	providerCode := strings.TrimSpace(string(account.ProviderCode))
	if providerCode == "" {
		providerCode = string(current.ProviderCode)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE provider_accounts
		SET
			display_name = ?,
			environment = ?,
			credential_json = ?,
			config_json = ?,
			provider_id = (SELECT id FROM providers WHERE code = ?),
			updated_at = ?
		WHERE id = ?
	`, displayName, environment, string(credentialJSON), string(configJSON), providerCode, time.Now().UTC().Format(time.RFC3339Nano), accountID)
	if err != nil {
		return "", fmt.Errorf("update provider account: %w", err)
	}
	return accountID, nil
}

func (s *Store) DeleteProviderAccountByID(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("provider account id is required")
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM provider_accounts
		WHERE id = ?
	`, accountID)
	if err != nil {
		return fmt.Errorf("delete provider account: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("provider account rows affected: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("provider account %q not found", accountID)
	}
	return nil
}

type providerAccountScanner interface {
	Scan(dest ...any) error
}

func scanProviderAccount(scanner providerAccountScanner) (domain.ProviderAccount, error) {
	var (
		account       domain.ProviderAccount
		providerCode  string
		environment   string
		credentialRaw string
		configRaw     string
		createdAtRaw  string
		updatedAtRaw  string
	)
	if err := scanner.Scan(&account.ID, &providerCode, &environment, &account.DisplayName, &credentialRaw, &configRaw, &createdAtRaw, &updatedAtRaw); err != nil {
		return domain.ProviderAccount{}, fmt.Errorf("scan provider account: %w", err)
	}
	account.ProviderCode = domain.ProviderCode(providerCode)
	account.Environment = domain.Environment(environment)
	account.CredentialJSON = json.RawMessage(credentialRaw)
	account.ConfigJSON = json.RawMessage(configRaw)
	account.CreatedAt = parseTime(createdAtRaw)
	account.UpdatedAt = parseTime(updatedAtRaw)
	return account, nil
}
