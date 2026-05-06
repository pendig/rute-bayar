package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

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
	processedAt := event.ProcessedAt
	if processedAt == nil {
		defaultProcessedAt := receivedAt
		processedAt = &defaultProcessedAt
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
			?,
			?
		)
	`, id, string(event.ProviderCode), nullable(event.ProviderEventID), eventType, boolInt(event.SignatureValid), string(event.PayloadJSON), string(event.HeadersJSON), receivedAt.Format(time.RFC3339Nano), processedAt.Format(time.RFC3339Nano), processingStatus)
	if err != nil {
		return "", fmt.Errorf("record webhook event: %w", err)
	}

	return id, nil
}

func (s *Store) GetWebhookEventByProviderEventID(ctx context.Context, provider domain.ProviderCode, providerEventID string) (domain.WebhookEvent, error) {
	var (
		event          domain.WebhookEvent
		providerCode   string
		signatureValid int
		receivedAt     string
		processedAt    sql.NullString
		payloadJSON    string
		headersJSON    string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			we.id,
			p.code,
			we.provider_event_id,
			we.event_type,
			we.signature_valid,
			we.payload_json,
			we.headers_json,
			we.received_at,
			we.processed_at,
			we.processing_status
		FROM webhook_events we
		JOIN providers p ON p.id = we.provider_id
		WHERE p.code = ?
		  AND we.provider_event_id = ?
		ORDER BY we.received_at DESC
		LIMIT 1
	`, string(provider), providerEventID).Scan(
		&event.ID,
		&providerCode,
		&event.ProviderEventID,
		&event.EventType,
		&signatureValid,
		&payloadJSON,
		&headersJSON,
		&receivedAt,
		&processedAt,
		&event.ProcessingStatus,
	)
	if err != nil {
		return domain.WebhookEvent{}, err
	}

	event.ProviderCode = domain.ProviderCode(providerCode)
	event.SignatureValid = signatureValid == 1
	event.PayloadJSON = []byte(payloadJSON)
	event.HeadersJSON = []byte(headersJSON)
	event.ReceivedAt = parseTime(receivedAt)
	if processedAt.Valid {
		parsed := parseTime(processedAt.String)
		event.ProcessedAt = &parsed
	}

	return event, nil
}

func (s *Store) GetWebhookEventByID(ctx context.Context, eventID string) (domain.WebhookEvent, error) {
	var (
		event          domain.WebhookEvent
		providerCode   string
		signatureValid int
		receivedAt     string
		processedAt    sql.NullString
		payloadJSON    string
		headersJSON    string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT
			we.id,
			p.code,
			we.provider_event_id,
			we.event_type,
			we.signature_valid,
			we.payload_json,
			we.headers_json,
			we.received_at,
			we.processed_at,
			we.processing_status
		FROM webhook_events we
		JOIN providers p ON p.id = we.provider_id
		WHERE we.id = ?
	`, eventID).Scan(
		&event.ID,
		&providerCode,
		&event.ProviderEventID,
		&event.EventType,
		&signatureValid,
		&payloadJSON,
		&headersJSON,
		&receivedAt,
		&processedAt,
		&event.ProcessingStatus,
	)
	if err != nil {
		return domain.WebhookEvent{}, err
	}

	event.ProviderCode = domain.ProviderCode(providerCode)
	event.SignatureValid = signatureValid == 1
	event.PayloadJSON = []byte(payloadJSON)
	event.HeadersJSON = []byte(headersJSON)
	event.ReceivedAt = parseTime(receivedAt)
	if processedAt.Valid {
		parsed := parseTime(processedAt.String)
		event.ProcessedAt = &parsed
	}

	return event, nil
}

func parseTime(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
