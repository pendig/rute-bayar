package memory

import (
	"context"
	"sync"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
)

type ForwardingStore struct {
	mu       sync.Mutex
	targets  []forwarding.Target
	attempts []forwarding.Attempt
}

func NewForwardingStore() *ForwardingStore {
	return &ForwardingStore{}
}

func (s *ForwardingStore) ListEnabledTargets(_ context.Context, provider domain.ProviderCode) ([]forwarding.Target, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	targets := make([]forwarding.Target, 0)
	for _, target := range s.targets {
		if target.Enabled && target.Provider == provider {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func (s *ForwardingStore) RecordAttempt(_ context.Context, attempt forwarding.Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.attempts = append(s.attempts, attempt)
	return nil
}
