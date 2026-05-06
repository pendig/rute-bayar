package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

func (s *Store) RecordRefund(ctx context.Context, refund domain.Refund) (string, error) {
	id := refund.ID
	if id == "" {
		id = newID("refund")
	}
	status := refund.Status
	if status == "" {
		status = domain.PaymentStatusPending
	}
	requestJSON := refund.RequestJSON
	if len(requestJSON) == 0 {
		requestJSON = []byte("{}")
	}
	responseJSON := refund.ResponseJSON
	if len(responseJSON) == 0 {
		responseJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO refunds (
			id,
			payment_intent_id,
			provider_id,
			amount,
			status,
			request_json,
			response_json,
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
			?,
			?
		)
	`, id, refund.PaymentIntentID, string(refund.ProviderCode), refund.Amount, string(status), string(requestJSON), string(responseJSON), nullable(refund.ProviderReference), now, now)
	if err != nil {
		return "", fmt.Errorf("record refund: %w", err)
	}

	return id, nil
}

func (s *Store) GetLatestRefundByIntent(ctx context.Context, paymentIntentID string, provider domain.ProviderCode) (domain.Refund, error) {
	var (
		refund       domain.Refund
		providerCode string
		requestRaw   string
		responseRaw  string
		createdAtRaw string
		updatedAtRaw string
		providerRef  sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			r.id,
			r.payment_intent_id,
			p.code,
			r.amount,
			r.status,
			r.request_json,
			r.response_json,
			r.provider_reference,
			r.created_at,
			r.updated_at
		FROM refunds r
		JOIN providers p ON p.id = r.provider_id
		WHERE r.payment_intent_id = ? AND p.code = ?
		ORDER BY r.created_at DESC
		LIMIT 1
	`, paymentIntentID, string(provider)).Scan(&refund.ID, &refund.PaymentIntentID, &providerCode, &refund.Amount, &refund.Status, &requestRaw, &responseRaw, &providerRef, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Refund{}, fmt.Errorf("%w: refund for %s/%s is not configured", sql.ErrNoRows, paymentIntentID, provider)
		}
		return domain.Refund{}, fmt.Errorf("get refund: %w", err)
	}

	refund.ProviderCode = domain.ProviderCode(providerCode)
	refund.RequestJSON = json.RawMessage(requestRaw)
	refund.ResponseJSON = json.RawMessage(responseRaw)
	refund.CreatedAt = parseTime(createdAtRaw)
	refund.UpdatedAt = parseTime(updatedAtRaw)
	if providerRef.Valid {
		refund.ProviderReference = providerRef.String
	}

	return refund, nil
}
