package sqlite

import (
	"context"
	"fmt"
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
	if len(target.ID) == 0 {
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
