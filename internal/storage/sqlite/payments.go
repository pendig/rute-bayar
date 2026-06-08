package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

func (s *Store) GetPaymentIntentByExternalRef(ctx context.Context, externalRef string) (domain.PaymentIntent, error) {
	var (
		intent       domain.PaymentIntent
		providerCode string
		metadataRaw  string
		createdAtRaw string
		updatedAtRaw string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			pi.id,
			pi.external_ref,
			p.code,
			pi.amount,
			pi.currency,
			pi.status,
			pi.metadata_json,
			pi.created_at,
			pi.updated_at
		FROM payment_intents pi
		JOIN providers p ON p.id = pi.provider_id
		WHERE pi.external_ref = ?
	`, externalRef).Scan(&intent.ID, &intent.ExternalRef, &providerCode, &intent.Amount, &intent.Currency, &intent.Status, &metadataRaw, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.PaymentIntent{}, fmt.Errorf("%w: payment intent %s is not configured", sql.ErrNoRows, externalRef)
		}
		return domain.PaymentIntent{}, fmt.Errorf("get payment intent: %w", err)
	}

	intent.ProviderCode = domain.ProviderCode(providerCode)
	intent.MetadataJSON = json.RawMessage(metadataRaw)
	intent.CreatedAt = parseTime(createdAtRaw)
	intent.UpdatedAt = parseTime(updatedAtRaw)
	return intent, nil
}

func (s *Store) GetLatestPaymentAttemptByIntent(ctx context.Context, paymentIntentID string, provider domain.ProviderCode) (domain.PaymentAttempt, error) {
	var (
		attempt      domain.PaymentAttempt
		providerCode string
		requestRaw   string
		responseRaw  string
		createdAtRaw string
		updatedAtRaw string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			pa.id,
			pa.payment_intent_id,
			p.code,
			pa.request_json,
			pa.response_json,
			pa.status,
			pa.provider_reference,
			pa.created_at,
			pa.updated_at
		FROM payment_attempts pa
		JOIN providers p ON p.id = pa.provider_id
		WHERE pa.payment_intent_id = ? AND p.code = ?
		ORDER BY pa.created_at DESC
		LIMIT 1
	`, paymentIntentID, string(provider)).Scan(&attempt.ID, &attempt.PaymentIntentID, &providerCode, &requestRaw, &responseRaw, &attempt.Status, &attempt.ProviderReference, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.PaymentAttempt{}, fmt.Errorf("%w: payment attempt for %s/%s is not configured", sql.ErrNoRows, paymentIntentID, provider)
		}
		return domain.PaymentAttempt{}, fmt.Errorf("get payment attempt: %w", err)
	}

	attempt.ProviderCode = domain.ProviderCode(providerCode)
	attempt.RequestJSON = json.RawMessage(requestRaw)
	attempt.ResponseJSON = json.RawMessage(responseRaw)
	attempt.CreatedAt = parseTime(createdAtRaw)
	attempt.UpdatedAt = parseTime(updatedAtRaw)
	return attempt, nil
}

func (s *Store) ListPaymentIntents(ctx context.Context, provider domain.ProviderCode, environment domain.Environment, status domain.PaymentStatus, limit, offset int) ([]domain.PaymentIntent, error) {
	_ = environment

	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	query := ""
	query += "\n\t\tSELECT\n\t\t\tpi.id,\n\t\t\tpi.external_ref,\n\t\t\tp.code,\n\t\t\tpi.amount,\n\t\t\tpi.currency,\n\t\t\tpi.status,\n\t\t\tpi.metadata_json,\n\t\t\tpi.created_at,\n\t\t\tpi.updated_at\n\t\tFROM payment_intents pi\n\t\tJOIN providers p ON p.id = pi.provider_id\n\t\tWHERE 1 = 1"
	args := make([]any, 0, 2)

	if provider != "" {
		query += " AND p.code = ?"
		args = append(args, string(provider))
	}
	if status != "" {
		query += " AND pi.status = ?"
		args = append(args, string(status))
	}
	query += " ORDER BY pi.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query payment intents: %w", err)
	}
	defer rows.Close()

	items := make([]domain.PaymentIntent, 0)
	for rows.Next() {
		var (
			intent       domain.PaymentIntent
			providerCode string
			metadataRaw  string
			createdAtRaw string
			updatedAtRaw string
		)
		if err := rows.Scan(&intent.ID, &intent.ExternalRef, &providerCode, &intent.Amount, &intent.Currency, &intent.Status, &metadataRaw, &createdAtRaw, &updatedAtRaw); err != nil {
			return nil, fmt.Errorf("scan payment intent: %w", err)
		}
		intent.ProviderCode = domain.ProviderCode(providerCode)
		intent.MetadataJSON = json.RawMessage(metadataRaw)
		intent.CreatedAt = parseTime(createdAtRaw)
		intent.UpdatedAt = parseTime(updatedAtRaw)
		items = append(items, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payment intents: %w", err)
	}

	return items, nil
}

func (s *Store) CountPaymentIntents(ctx context.Context, provider domain.ProviderCode, environment domain.Environment, status domain.PaymentStatus) (int, error) {
	_ = environment

	query := ""
	query += "\n\t\tSELECT COUNT(1)\n\t\tFROM payment_intents pi\n\t\tJOIN providers p ON p.id = pi.provider_id\n\t\tWHERE 1 = 1"
	args := make([]any, 0, 2)

	if provider != "" {
		query += " AND p.code = ?"
		args = append(args, string(provider))
	}
	if status != "" {
		query += " AND pi.status = ?"
		args = append(args, string(status))
	}

	var total int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count payment intents: %w", err)
	}
	return total, nil
}

func (s *Store) UpsertPaymentIntent(ctx context.Context, intent domain.PaymentIntent) (string, error) {
	id := intent.ID
	if id == "" {
		id = newID("payment_intent")
	}
	metadataJSON := intent.MetadataJSON
	if len(metadataJSON) == 0 {
		metadataJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payment_intents (
			id,
			external_ref,
			provider_id,
			amount,
			currency,
			status,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			?,
			?,
			(SELECT id FROM providers WHERE code = ?),
			?,
			?,
			?,
			?,
			?,
			?
		)
		ON CONFLICT(external_ref)
		DO UPDATE SET
			provider_id = excluded.provider_id,
			amount = excluded.amount,
			currency = excluded.currency,
			status = excluded.status,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`, id, intent.ExternalRef, string(intent.ProviderCode), intent.Amount, intent.Currency, string(intent.Status), string(metadataJSON), now, now)
	if err != nil {
		return "", fmt.Errorf("upsert payment intent: %w", err)
	}

	var savedID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM payment_intents
		WHERE external_ref = ?
	`, intent.ExternalRef).Scan(&savedID); err != nil {
		return "", fmt.Errorf("load payment intent id: %w", err)
	}

	return savedID, nil
}

func (s *Store) RecordPaymentAttempt(ctx context.Context, attempt domain.PaymentAttempt) (string, error) {
	id := attempt.ID
	if id == "" {
		id = newID("payment_attempt")
	}
	requestJSON := attempt.RequestJSON
	if len(requestJSON) == 0 {
		requestJSON = []byte("{}")
	}
	responseJSON := attempt.ResponseJSON
	if len(responseJSON) == 0 {
		responseJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payment_attempts (
			id,
			payment_intent_id,
			provider_id,
			request_json,
			response_json,
			status,
			provider_reference,
			created_at,
			updated_at
		)
		VALUES (
			?,
			?,
			(SELECT id FROM providers WHERE code = ?),
			?,
			?,
			?,
			?,
			?,
			?
		)
	`, id, attempt.PaymentIntentID, string(attempt.ProviderCode), string(requestJSON), string(responseJSON), string(attempt.Status), attempt.ProviderReference, now, now)
	if err != nil {
		return "", fmt.Errorf("record payment attempt: %w", err)
	}

	return id, nil
}

func (s *Store) RecordPaymentStatusCheck(ctx context.Context, check domain.PaymentStatusCheck) (string, error) {
	id := check.ID
	if id == "" {
		id = newID("payment_status_check")
	}
	requestJSON := check.RequestJSON
	if len(requestJSON) == 0 {
		requestJSON = []byte("{}")
	}
	responseJSON := check.ResponseJSON
	if len(responseJSON) == 0 {
		responseJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payment_status_checks (
			id,
			payment_intent_id,
			provider_id,
			request_json,
			response_json,
			status,
			provider_reference,
			created_at,
			updated_at
		)
		VALUES (
			?,
			?,
			(SELECT id FROM providers WHERE code = ?),
			?,
			?,
			?,
			?,
			?,
			?
		)
	`, id, check.PaymentIntentID, string(check.ProviderCode), string(requestJSON), string(responseJSON), string(check.Status), check.ProviderReference, now, now)
	if err != nil {
		return "", fmt.Errorf("record payment status check: %w", err)
	}

	return id, nil
}
