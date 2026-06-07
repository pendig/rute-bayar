package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pendig/rute-bayar/internal/auditlog"
)

func (s *Store) RecordAuditEvent(ctx context.Context, event auditlog.Event) error {
	if s == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	detail, _ := json.Marshal(map[string]any{
		"request_id":  event.RequestID,
		"duration_ms": event.DurationMs,
		"client_ip":   event.ClientIP,
	})

	id := newID("audit")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id,
			actor_type,
			actor_id,
			action,
			target_type,
			target_id,
			detail_json,
			created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
		event.ActorType,
		event.ActorID,
		event.Method+" "+event.Path,
		"api",
		event.Path,
		detail,
		now,
	)
	if err != nil {
		return err
	}

	return nil
}
