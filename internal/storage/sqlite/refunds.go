package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

func (s *Store) UpdateRefundStatusByProviderIdentifiers(ctx context.Context, providerCode domain.ProviderCode, identifiers []string, status domain.PaymentStatus, responseJSON json.RawMessage) (domain.Refund, error) {
	cleaned := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if trimmed := strings.TrimSpace(identifier); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return domain.Refund{}, fmt.Errorf("%w: refund identifier is required", sql.ErrNoRows)
	}

	clauses := make([]string, 0, len(cleaned))
	args := []any{string(providerCode)}
	for _, identifier := range cleaned {
		clauses = append(clauses, "(r.provider_reference = ? OR r.request_json LIKE ? OR r.response_json LIKE ?)")
		args = append(args, identifier, "%"+identifier+"%", "%"+identifier+"%")
	}

	query := fmt.Sprintf(`
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
		WHERE p.code = ? AND (%s)
		ORDER BY r.created_at DESC
		LIMIT 1
	`, strings.Join(clauses, " OR "))

	refund, err := scanRefund(s.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Refund{}, fmt.Errorf("%w: refund for %s is not configured", sql.ErrNoRows, strings.Join(cleaned, ","))
		}
		return domain.Refund{}, err
	}

	nextResponseJSON := refund.ResponseJSON
	if len(responseJSON) > 0 {
		nextResponseJSON = responseJSON
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		UPDATE refunds
		SET status = ?, response_json = ?, updated_at = ?
		WHERE id = ?
	`, string(status), string(nextResponseJSON), now, refund.ID)
	if err != nil {
		return domain.Refund{}, fmt.Errorf("update refund status: %w", err)
	}

	refund.Status = status
	refund.ResponseJSON = nextResponseJSON
	refund.UpdatedAt = parseTime(now)
	return refund, nil
}

type refundScanner interface {
	Scan(dest ...any) error
}

func scanRefund(scanner refundScanner) (domain.Refund, error) {
	var (
		refund       domain.Refund
		providerCode string
		requestRaw   string
		responseRaw  string
		createdAtRaw string
		updatedAtRaw string
		providerRef  sql.NullString
	)

	err := scanner.Scan(&refund.ID, &refund.PaymentIntentID, &providerCode, &refund.Amount, &refund.Status, &requestRaw, &responseRaw, &providerRef, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		return domain.Refund{}, err
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
