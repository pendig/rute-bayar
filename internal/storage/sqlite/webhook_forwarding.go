package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
)

func (s *Store) ListEnabledTargets(ctx context.Context, provider domain.ProviderCode) ([]forwarding.Target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			name,
			target_url,
			auth_json,
			event_filter_json,
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
			eventFilterJSON string
			retryPolicyJSON string
			enabled         int
		)
		if err := rows.Scan(&target.ID, &target.Name, &target.URL, &authJSON, &eventFilterJSON, &retryPolicyJSON, &enabled); err != nil {
			return nil, fmt.Errorf("scan forwarding target: %w", err)
		}

		target.Provider = provider
		target.Enabled = enabled == 1
		target.Headers = headersFromJSON(authJSON)
		target.EventFilter = eventFilterFromJSON(eventFilterJSON)
		target.RetryPolicy = retryPolicyFromJSON(retryPolicyJSON)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forwarding targets: %w", err)
	}

	return targets, nil
}

func (s *Store) AddForwardingTarget(ctx context.Context, target forwarding.Target) (string, error) {
	id := target.ID
	if id == "" {
		id = newID("fwd_target")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	authJSON := headersToJSON(target.Headers)
	eventFilterJSON := eventFilterToJSON(target.EventFilter)
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
		VALUES (?, ?, (SELECT id FROM providers WHERE code = ?), ?, ?, ?, ?, ?, ?, ?)
	`, id, target.Name, string(target.Provider), eventFilterJSON, target.URL, authJSON, retryPolicyJSON, enabled, now, now)
	if err != nil {
		return "", fmt.Errorf("add forwarding target: %w", err)
	}

	return id, nil
}

func (s *Store) ListForwardingTargets(ctx context.Context, provider domain.ProviderCode) ([]forwarding.Target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			name,
			target_url,
			auth_json,
			event_filter_json,
			retry_policy_json,
			enabled
		FROM webhook_forwarding_targets
		WHERE provider_id = (SELECT id FROM providers WHERE code = ?)
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
			eventFilterJSON string
			retryPolicyJSON string
			enabled         int
		)
		if err := rows.Scan(&target.ID, &target.Name, &target.URL, &authJSON, &eventFilterJSON, &retryPolicyJSON, &enabled); err != nil {
			return nil, fmt.Errorf("scan forwarding target: %w", err)
		}

		target.Provider = provider
		target.Enabled = enabled == 1
		target.Headers = headersFromJSON(authJSON)
		target.EventFilter = eventFilterFromJSON(eventFilterJSON)
		target.RetryPolicy = retryPolicyFromJSON(retryPolicyJSON)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forwarding targets: %w", err)
	}

	return targets, nil
}

func (s *Store) GetForwardingTarget(ctx context.Context, targetID string) (forwarding.Target, error) {
	var (
		target          forwarding.Target
		providerCode    string
		authJSON        string
		eventFilterJSON string
		retryPolicyJSON string
		enabled         int
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			wt.id,
			p.code,
			wt.name,
			wt.target_url,
			wt.auth_json,
			wt.event_filter_json,
			wt.retry_policy_json,
			wt.enabled
		FROM webhook_forwarding_targets wt
		JOIN providers p ON p.id = wt.provider_id
		WHERE wt.id = ?
	`, targetID).Scan(&target.ID, &providerCode, &target.Name, &target.URL, &authJSON, &eventFilterJSON, &retryPolicyJSON, &enabled)
	if err != nil {
		return forwarding.Target{}, fmt.Errorf("get forwarding target: %w", err)
	}

	target.Provider = domain.ProviderCode(providerCode)
	target.Enabled = enabled == 1
	target.Headers = headersFromJSON(authJSON)
	target.EventFilter = eventFilterFromJSON(eventFilterJSON)
	target.RetryPolicy = retryPolicyFromJSON(retryPolicyJSON)
	return target, nil
}

func (s *Store) UpdateForwardingTarget(ctx context.Context, target forwarding.Target) error {
	if strings.TrimSpace(target.ID) == "" {
		return fmt.Errorf("forwarding target id is required")
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE webhook_forwarding_targets
		SET
			name = ?,
			target_url = ?,
			auth_json = ?,
			event_filter_json = ?,
			retry_policy_json = ?,
			enabled = ?,
			updated_at = ?
		WHERE id = ?
	`, target.Name, target.URL, headersToJSON(target.Headers), eventFilterToJSON(target.EventFilter), retryPolicyToJSON(target.RetryPolicy), boolInt(target.Enabled), time.Now().UTC().Format(time.RFC3339Nano), target.ID)
	if err != nil {
		return fmt.Errorf("update forwarding target: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check forwarding target update: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("forwarding target %q not found", target.ID)
	}
	return nil
}

func (s *Store) DeleteForwardingTarget(ctx context.Context, targetID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM webhook_forwarding_targets
		WHERE id = ?
	`, targetID)
	if err != nil {
		return fmt.Errorf("delete forwarding target: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check forwarding target delete: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("forwarding target %q not found", targetID)
	}
	return nil
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

func (s *Store) ListForwardingAttempts(ctx context.Context, filter forwarding.AttemptFilter) ([]forwarding.AttemptRecord, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var query strings.Builder
	query.WriteString(`
		SELECT
			wfa.id,
			wfa.webhook_event_id,
			wfa.forwarding_target_id,
			wft.name,
			wft.target_url,
			p.code,
			wfa.request_json,
			wfa.response_json,
			wfa.status,
			wfa.attempt_no,
			wfa.created_at,
			wfa.updated_at
		FROM webhook_forwarding_attempts wfa
		JOIN webhook_forwarding_targets wft ON wft.id = wfa.forwarding_target_id
		JOIN providers p ON p.id = wft.provider_id
		WHERE 1 = 1
	`)

	args := make([]any, 0, 5)
	if filter.Provider != "" {
		query.WriteString(" AND p.code = ?")
		args = append(args, string(filter.Provider))
	}
	if strings.TrimSpace(filter.TargetID) != "" {
		query.WriteString(" AND wfa.forwarding_target_id = ?")
		args = append(args, strings.TrimSpace(filter.TargetID))
	}
	if strings.TrimSpace(filter.WebhookEventID) != "" {
		query.WriteString(" AND wfa.webhook_event_id = ?")
		args = append(args, strings.TrimSpace(filter.WebhookEventID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query.WriteString(" AND wfa.status = ?")
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query.WriteString(" ORDER BY wfa.created_at DESC LIMIT ?")
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query forwarding attempts: %w", err)
	}
	defer rows.Close()

	attempts := make([]forwarding.AttemptRecord, 0)
	for rows.Next() {
		attempt, err := scanForwardingAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forwarding attempts: %w", err)
	}
	return attempts, nil
}

func (s *Store) CountForwardingAttempts(ctx context.Context, filter forwarding.AttemptFilter) (int, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT COUNT(1)
		FROM webhook_forwarding_attempts wfa
		JOIN webhook_forwarding_targets wft ON wft.id = wfa.forwarding_target_id
		JOIN providers p ON p.id = wft.provider_id
		WHERE 1 = 1
	`)
	args := make([]any, 0, 4)
	if filter.Provider != "" {
		query.WriteString(" AND p.code = ?")
		args = append(args, string(filter.Provider))
	}
	if strings.TrimSpace(filter.TargetID) != "" {
		query.WriteString(" AND wfa.forwarding_target_id = ?")
		args = append(args, strings.TrimSpace(filter.TargetID))
	}
	if strings.TrimSpace(filter.WebhookEventID) != "" {
		query.WriteString(" AND wfa.webhook_event_id = ?")
		args = append(args, strings.TrimSpace(filter.WebhookEventID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query.WriteString(" AND wfa.status = ?")
		args = append(args, strings.TrimSpace(filter.Status))
	}

	var total int
	if err := s.db.QueryRowContext(ctx, query.String(), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count forwarding attempts: %w", err)
	}

	return total, nil
}

func (s *Store) CountForwardingTargets(ctx context.Context, provider domain.ProviderCode, includeDisabled bool) (int, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT COUNT(1)
		FROM webhook_forwarding_targets wt
		JOIN providers p ON p.id = wt.provider_id
		WHERE 1 = 1
	`)
	args := make([]any, 0, 2)
	if provider != "" {
		query.WriteString(" AND p.code = ?")
		args = append(args, string(provider))
	}
	if !includeDisabled {
		query.WriteString(" AND wt.enabled = 1")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, query.String(), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count forwarding targets: %w", err)
	}

	return total, nil
}

func (s *Store) GetForwardingAttempt(ctx context.Context, attemptID string) (forwarding.AttemptRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			wfa.id,
			wfa.webhook_event_id,
			wfa.forwarding_target_id,
			wft.name,
			wft.target_url,
			p.code,
			wfa.request_json,
			wfa.response_json,
			wfa.status,
			wfa.attempt_no,
			wfa.created_at,
			wfa.updated_at
		FROM webhook_forwarding_attempts wfa
		JOIN webhook_forwarding_targets wft ON wft.id = wfa.forwarding_target_id
		JOIN providers p ON p.id = wft.provider_id
		WHERE wfa.id = ?
	`, strings.TrimSpace(attemptID))

	attempt, err := scanForwardingAttempt(row)
	if err != nil {
		return forwarding.AttemptRecord{}, fmt.Errorf("get forwarding attempt: %w", err)
	}
	return attempt, nil
}

type forwardingAttemptScanner interface {
	Scan(dest ...any) error
}

func scanForwardingAttempt(scanner forwardingAttemptScanner) (forwarding.AttemptRecord, error) {
	var (
		attempt      forwarding.AttemptRecord
		providerCode string
		requestJSON  string
		responseJSON string
		createdAt    string
		updatedAt    string
	)
	if err := scanner.Scan(
		&attempt.ID,
		&attempt.WebhookEventID,
		&attempt.TargetID,
		&attempt.TargetName,
		&attempt.TargetURL,
		&providerCode,
		&requestJSON,
		&responseJSON,
		&attempt.Status,
		&attempt.AttemptNo,
		&createdAt,
		&updatedAt,
	); err != nil {
		return forwarding.AttemptRecord{}, fmt.Errorf("scan forwarding attempt: %w", err)
	}

	attempt.Provider = domain.ProviderCode(providerCode)
	attempt.RequestJSON = []byte(requestJSON)
	attempt.ResponseJSON = []byte(responseJSON)
	attempt.CreatedAt = parseTime(createdAt)
	attempt.UpdatedAt = parseTime(updatedAt)
	return attempt, nil
}
