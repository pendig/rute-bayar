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

func (s *Store) ReconcileRefundByProviderIdentifiers(ctx context.Context, providerCode domain.ProviderCode, identifiers []string, status domain.PaymentStatus, responseJSON json.RawMessage) (domain.Refund, error) {
	cleaned := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if trimmed := strings.TrimSpace(identifier); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return domain.Refund{}, fmt.Errorf("%w: refund identifier is required", sql.ErrNoRows)
	}

	matchClause, matchArgs := refundIdentifierMatchClause(cleaned)
	args := append([]any{string(providerCode)}, matchArgs...)
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
	`, matchClause)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Refund{}, fmt.Errorf("begin refund reconciliation transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	refund, err := scanRefund(tx.QueryRowContext(ctx, query, args...))
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
	result, err := tx.ExecContext(ctx, `
		UPDATE refunds
		SET status = ?, response_json = ?, updated_at = ?
		WHERE id = ?
	`, string(status), string(nextResponseJSON), now, refund.ID)
	if err != nil {
		return domain.Refund{}, fmt.Errorf("update refund status: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil {
		return domain.Refund{}, fmt.Errorf("read refund status update rows affected: %w", err)
	} else if rows == 0 {
		return domain.Refund{}, fmt.Errorf("%w: refund %s is not configured", sql.ErrNoRows, refund.ID)
	}

	if status == domain.PaymentStatusRefunded || status == domain.PaymentStatusPartialRefunded {
		result, err = tx.ExecContext(ctx, `
			UPDATE payment_intents
			SET status = ?, updated_at = ?
			WHERE id = ?
		`, string(status), now, refund.PaymentIntentID)
		if err != nil {
			return domain.Refund{}, fmt.Errorf("update refunded payment intent status: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil {
			return domain.Refund{}, fmt.Errorf("read refunded payment intent update rows affected: %w", err)
		} else if rows == 0 {
			return domain.Refund{}, fmt.Errorf("%w: payment intent %s is not configured", sql.ErrNoRows, refund.PaymentIntentID)
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Refund{}, fmt.Errorf("commit refund reconciliation transaction: %w", err)
	}
	committed = true

	refund.Status = status
	refund.ResponseJSON = nextResponseJSON
	refund.UpdatedAt = parseTime(now)
	return refund, nil
}

func refundIdentifierMatchClause(identifiers []string) (string, []any) {
	placeholders := sqlPlaceholders(len(identifiers))
	fields := []string{
		"r.provider_reference",
		"json_extract(r.request_json, '$.reference_id')",
		"json_extract(r.request_json, '$.payment_request_id')",
		"json_extract(r.request_json, '$.id')",
		"json_extract(r.response_json, '$.reference_id')",
		"json_extract(r.response_json, '$.payment_request_id')",
		"json_extract(r.response_json, '$.payment_id')",
		"json_extract(r.response_json, '$.invoice_id')",
		"json_extract(r.response_json, '$.id')",
		"json_extract(r.response_json, '$.data.reference_id')",
		"json_extract(r.response_json, '$.data.payment_request_id')",
		"json_extract(r.response_json, '$.data.payment_id')",
		"json_extract(r.response_json, '$.data.invoice_id')",
		"json_extract(r.response_json, '$.data.id')",
	}

	clauses := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)*len(identifiers))
	for _, field := range fields {
		clauses = append(clauses, field+" IN ("+placeholders+")")
		for _, identifier := range identifiers {
			args = append(args, identifier)
		}
	}
	return strings.Join(clauses, " OR "), args
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
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
