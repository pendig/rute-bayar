package sqlite

import (
	"context"
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
