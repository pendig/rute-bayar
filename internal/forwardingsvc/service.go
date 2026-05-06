package forwardingsvc

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/forwarding"
)

type Store interface {
	ListForwardingTargets(context.Context, domain.ProviderCode) ([]forwarding.Target, error)
	AddForwardingTarget(context.Context, forwarding.Target) (string, error)
	GetForwardingTarget(context.Context, string) (forwarding.Target, error)
	UpdateForwardingTarget(context.Context, forwarding.Target) error
	DeleteForwardingTarget(context.Context, string) error
}

type Service struct {
	store Store
}

type ListInput struct {
	Provider        domain.ProviderCode
	IncludeDisabled bool
}

type AddInput struct {
	Provider    domain.ProviderCode
	Name        string
	URL         string
	Enabled     bool
	Headers     http.Header
	EventFilter map[string]string
	RetryPolicy forwarding.RetryPolicy
}

type UpdateInput struct {
	ID string

	Name string
	URL  string

	Enabled    bool
	EnabledSet bool

	Headers    http.Header
	HeadersSet bool

	EventFilter    map[string]string
	EventFilterSet bool

	RetryPolicy    forwarding.RetryPolicy
	RetryPolicySet bool
}

func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, input ListInput) ([]forwarding.Target, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("forwarding service is not configured")
	}
	providers, err := normalizeProviders(input.Provider)
	if err != nil {
		return nil, err
	}

	var targets []forwarding.Target
	for _, providerCode := range providers {
		items, err := s.store.ListForwardingTargets(ctx, providerCode)
		if err != nil {
			return nil, err
		}
		for _, target := range items {
			if !input.IncludeDisabled && !target.Enabled {
				continue
			}
			targets = append(targets, target)
		}
	}

	return targets, nil
}

func (s *Service) Add(ctx context.Context, input AddInput) (string, error) {
	if s == nil || s.store == nil {
		return "", fmt.Errorf("forwarding service is not configured")
	}
	providerCode, err := normalizeProvider(input.Provider)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	targetURL := strings.TrimSpace(input.URL)
	if targetURL == "" {
		return "", fmt.Errorf("url is required")
	}
	if err := validateRetryPolicyInput(input.RetryPolicy); err != nil {
		return "", err
	}
	policy := applyRetryDefaults(input.RetryPolicy)
	if err := validateRetryPolicy(policy); err != nil {
		return "", err
	}

	return s.store.AddForwardingTarget(ctx, forwarding.Target{
		Name:        name,
		Provider:    providerCode,
		URL:         targetURL,
		Headers:     cloneHeaders(input.Headers),
		EventFilter: copyStringMap(input.EventFilter),
		RetryPolicy: policy,
		Enabled:     input.Enabled,
	})
}

func (s *Service) Update(ctx context.Context, input UpdateInput) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("forwarding service is not configured")
	}
	targetID := strings.TrimSpace(input.ID)
	if targetID == "" {
		return fmt.Errorf("target id is required")
	}

	target, err := s.store.GetForwardingTarget(ctx, targetID)
	if err != nil {
		return err
	}

	if trimmedName := strings.TrimSpace(input.Name); trimmedName != "" {
		target.Name = trimmedName
	}
	if trimmedURL := strings.TrimSpace(input.URL); trimmedURL != "" {
		target.URL = trimmedURL
	}
	if input.EnabledSet {
		target.Enabled = input.Enabled
	}
	if input.HeadersSet {
		target.Headers = cloneHeaders(input.Headers)
	}
	if input.EventFilterSet {
		target.EventFilter = copyStringMap(input.EventFilter)
	}
	if input.RetryPolicySet {
		if err := validateRetryPolicyInput(input.RetryPolicy); err != nil {
			return err
		}
		target.RetryPolicy = applyRetryDefaults(input.RetryPolicy)
	}

	if strings.TrimSpace(target.Name) == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.TrimSpace(target.URL) == "" {
		return fmt.Errorf("target url cannot be empty")
	}
	policy := applyRetryDefaults(target.RetryPolicy)
	if err := validateRetryPolicy(policy); err != nil {
		return err
	}

	target.RetryPolicy = policy
	return s.store.UpdateForwardingTarget(ctx, target)
}

func (s *Service) Remove(ctx context.Context, targetID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("forwarding service is not configured")
	}
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return fmt.Errorf("target id is required")
	}
	return s.store.DeleteForwardingTarget(ctx, targetID)
}

func normalizeProvider(value domain.ProviderCode) (domain.ProviderCode, error) {
	switch domain.ProviderCode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case domain.ProviderMidtrans:
		return domain.ProviderMidtrans, nil
	case domain.ProviderXendit:
		return domain.ProviderXendit, nil
	default:
		return "", fmt.Errorf("provider must be one of %q or %q", domain.ProviderMidtrans, domain.ProviderXendit)
	}
}

func normalizeProviders(value domain.ProviderCode) ([]domain.ProviderCode, error) {
	if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
		providerCode, err := normalizeProvider(domain.ProviderCode(trimmed))
		if err != nil {
			return nil, err
		}
		return []domain.ProviderCode{providerCode}, nil
	}
	return domain.SupportedProviders(), nil
}

func applyRetryDefaults(policy forwarding.RetryPolicy) forwarding.RetryPolicy {
	defaults := forwarding.DefaultRetryPolicy()
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaults.MaxAttempts
	}
	if policy.Timeout <= 0 {
		policy.Timeout = defaults.Timeout
	}
	if policy.Backoff < 0 {
		policy.Backoff = defaults.Backoff
	}
	return policy
}

func validateRetryPolicy(policy forwarding.RetryPolicy) error {
	if policy.MaxAttempts <= 0 {
		return fmt.Errorf("retry max attempts must be greater than zero")
	}
	if policy.Timeout <= 0 {
		return fmt.Errorf("retry timeout must be greater than zero")
	}
	if policy.Backoff < 0 {
		return fmt.Errorf("retry backoff cannot be negative")
	}
	return nil
}

func validateRetryPolicyInput(policy forwarding.RetryPolicy) error {
	if policy.MaxAttempts < 0 {
		return fmt.Errorf("retry max attempts cannot be negative")
	}
	if policy.Timeout < 0 {
		return fmt.Errorf("retry timeout cannot be negative")
	}
	if policy.Backoff < 0 {
		return fmt.Errorf("retry backoff cannot be negative")
	}
	return nil
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	return headers.Clone()
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
