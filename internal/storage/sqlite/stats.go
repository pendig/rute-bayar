package sqlite

import (
	"context"
	"fmt"
)

func (s *Store) CountProviderAccounts(ctx context.Context) (int, error) {
	return s.countRows(ctx, "provider_accounts")
}

func (s *Store) CountPaymentIntents(ctx context.Context) (int, error) {
	return s.countRows(ctx, "payment_intents")
}

func (s *Store) CountWebhookEvents(ctx context.Context) (int, error) {
	return s.countRows(ctx, "webhook_events")
}

func (s *Store) CountForwardingTargets(ctx context.Context) (int, error) {
	return s.countRows(ctx, "webhook_forwarding_targets")
}

func (s *Store) CountForwardingAttempts(ctx context.Context) (int, error) {
	return s.countRows(ctx, "webhook_forwarding_attempts")
}

func (s *Store) countRows(ctx context.Context, table string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}
