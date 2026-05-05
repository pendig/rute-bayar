package forwarding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
)

type Target struct {
	ID          string
	Name        string
	Provider    domain.ProviderCode
	EventFilter map[string]string
	URL         string
	Headers     http.Header
	RetryPolicy RetryPolicy
	Enabled     bool
}

type RetryPolicy struct {
	MaxAttempts int
	Timeout     time.Duration
	Backoff     time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Timeout:     10 * time.Second,
		Backoff:     2 * time.Second,
	}
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	defaults := DefaultRetryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.Timeout <= 0 {
		p.Timeout = defaults.Timeout
	}
	if p.Backoff <= 0 {
		p.Backoff = defaults.Backoff
	}
	return p
}

type InboundWebhook struct {
	WebhookEventID string
	Provider       domain.ProviderCode
	Headers        http.Header
	Body           []byte
}

type Attempt struct {
	WebhookEventID string
	TargetID       string
	AttemptNo      int
	RequestJSON    []byte
	ResponseJSON   []byte
	Status         string
}

type TargetStore interface {
	ListEnabledTargets(context.Context, domain.ProviderCode) ([]Target, error)
	RecordAttempt(context.Context, Attempt) error
}

type Service struct {
	client *http.Client
	store  TargetStore
}

func NewService(store TargetStore) *Service {
	return &Service{
		client: &http.Client{},
		store:  store,
	}
}

func (s *Service) Forward(ctx context.Context, inbound InboundWebhook) error {
	if s == nil || s.store == nil {
		return nil
	}

	targets, err := s.store.ListEnabledTargets(ctx, inbound.Provider)
	if err != nil {
		return fmt.Errorf("list forwarding targets: %w", err)
	}

	for _, target := range targets {
		if err := s.forwardToTarget(ctx, target, inbound); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) forwardToTarget(ctx context.Context, target Target, inbound InboundWebhook) error {
	policy := target.RetryPolicy.withDefaults()

	var lastErr error
	for attemptNo := 1; attemptNo <= policy.MaxAttempts; attemptNo++ {
		attemptCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
		responseJSON, err := s.send(attemptCtx, target, inbound)
		cancel()

		status := "success"
		if err != nil {
			status = "failed"
			lastErr = err
		}

		_ = s.store.RecordAttempt(ctx, Attempt{
			WebhookEventID: inbound.WebhookEventID,
			TargetID:       target.ID,
			AttemptNo:      attemptNo,
			RequestJSON:    marshalForwardRequest(target, inbound),
			ResponseJSON:   responseJSON,
			Status:         status,
		})

		if err == nil {
			return nil
		}

		if attemptNo < policy.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(policy.Backoff):
			}
		}
	}

	return fmt.Errorf("forward webhook to %q: %w", target.Name, lastErr)
}

func (s *Service) send(ctx context.Context, target Target, inbound InboundWebhook) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(inbound.Body))
	if err != nil {
		return nil, err
	}

	req.Header = inbound.Headers.Clone()
	for key, values := range target.Headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	responseJSON, _ := json.Marshal(map[string]any{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
		"body":        string(body),
	})
	if readErr != nil {
		return responseJSON, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseJSON, fmt.Errorf("target returned status %d", resp.StatusCode)
	}
	return responseJSON, nil
}

func marshalForwardRequest(target Target, inbound InboundWebhook) []byte {
	payload, _ := json.Marshal(map[string]any{
		"target_url": target.URL,
		"headers":    inbound.Headers,
		"body":       string(inbound.Body),
	})
	return payload
}
