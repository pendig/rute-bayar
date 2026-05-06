package providerfactory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider"
	"github.com/pendig/rute-bayar/internal/provider/midtrans"
	"github.com/pendig/rute-bayar/internal/provider/xendit"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

type AccountLoader interface {
	GetProviderAccount(context.Context, domain.ProviderCode, domain.Environment) (domain.ProviderAccount, error)
}

type Factory struct {
	loader     AccountLoader
	httpClient *http.Client
}

type Option func(*Factory)

func New(loader AccountLoader, options ...Option) *Factory {
	factory := &Factory{loader: loader}
	for _, option := range options {
		option(factory)
	}
	return factory
}

func WithHTTPClient(client *http.Client) Option {
	return func(factory *Factory) {
		if client != nil {
			factory.httpClient = client
		}
	}
}

func (f *Factory) AdapterForStoredAccount(ctx context.Context, providerCode domain.ProviderCode, environment domain.Environment, baseURL string) (provider.Adapter, error) {
	switch providerCode {
	case domain.ProviderMidtrans:
		adapter, err := f.MidtransAdapterForStoredAccount(ctx, environment, baseURL)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	case domain.ProviderXendit:
		adapter, err := f.XenditAdapterForStoredAccount(ctx, environment, baseURL)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	default:
		return nil, fmt.Errorf("provider %q is not implemented yet", providerCode)
	}
}

func (f *Factory) AdapterForAccount(account domain.ProviderAccount, baseURL string) (provider.Adapter, error) {
	switch account.ProviderCode {
	case domain.ProviderMidtrans:
		adapter, err := f.MidtransAdapterForAccount(account, baseURL)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	case domain.ProviderXendit:
		adapter, err := f.XenditAdapterForAccount(account, baseURL)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	default:
		return nil, fmt.Errorf("provider %q is not implemented yet", account.ProviderCode)
	}
}

func (f *Factory) WebhookHandlers(ctx context.Context, environment domain.Environment) (map[domain.ProviderCode]provider.Adapter, error) {
	if f == nil || f.loader == nil {
		return nil, fmt.Errorf("provider account loader is required")
	}

	handlers := make(map[domain.ProviderCode]provider.Adapter)
	for _, providerCode := range []domain.ProviderCode{domain.ProviderMidtrans, domain.ProviderXendit} {
		account, err := f.loader.GetProviderAccount(ctx, providerCode, environment)
		if err != nil {
			if errors.Is(err, sqlite.ErrProviderAccountNotConfigured) {
				continue
			}
			return nil, fmt.Errorf("load %s webhook account: %w", providerCode, err)
		}

		adapter, err := f.AdapterForAccount(account, "")
		if err != nil {
			return nil, fmt.Errorf("build %s webhook handler: %w", providerCode, err)
		}
		handlers[providerCode] = adapter
	}

	return handlers, nil
}

func (f *Factory) MidtransAdapterForStoredAccount(ctx context.Context, environment domain.Environment, baseURL string) (*midtrans.Adapter, error) {
	account, err := f.loadAccount(ctx, domain.ProviderMidtrans, environment)
	if err != nil {
		return nil, err
	}
	return f.MidtransAdapterForAccount(account, baseURL)
}

func (f *Factory) XenditAdapterForStoredAccount(ctx context.Context, environment domain.Environment, baseURL string) (*xendit.Adapter, error) {
	account, err := f.loadAccount(ctx, domain.ProviderXendit, environment)
	if err != nil {
		return nil, err
	}
	return f.XenditAdapterForAccount(account, baseURL)
}

func (f *Factory) MidtransAdapterForAccount(account domain.ProviderAccount, baseURL string) (*midtrans.Adapter, error) {
	credential, err := midtransCredentialFromJSON(account.CredentialJSON)
	if err != nil {
		return nil, err
	}

	var httpClient *http.Client
	if f != nil {
		httpClient = f.httpClient
	}

	options := []midtrans.Option{midtrans.WithServerKey(credential.ServerKey)}
	if client := httpClient; client != nil {
		options = append(options, midtrans.WithHTTPClient(client))
	}
	if trimmedBaseURL := strings.TrimSpace(baseURL); trimmedBaseURL != "" {
		options = append(options, midtrans.WithBaseURL(trimmedBaseURL))
	} else {
		options = append(options, midtrans.WithBaseURL(midtrans.BaseURLForEnvironment(account.Environment)))
	}
	return midtrans.New(options...), nil
}

func (f *Factory) XenditAdapterForAccount(account domain.ProviderAccount, baseURL string) (*xendit.Adapter, error) {
	secretKey, err := xenditSecretKeyFromJSON(account.CredentialJSON)
	if err != nil {
		return nil, err
	}

	var httpClient *http.Client
	if f != nil {
		httpClient = f.httpClient
	}

	options := []xendit.Option{xendit.WithSecretKey(secretKey)}
	if client := httpClient; client != nil {
		options = append(options, xendit.WithHTTPClient(client))
	}
	if trimmedBaseURL := strings.TrimSpace(baseURL); trimmedBaseURL != "" {
		options = append(options, xendit.WithBaseURL(trimmedBaseURL))
	}
	if callbackToken, err := xenditWebhookTokenFromConfig(account.ConfigJSON); err != nil {
		return nil, err
	} else if callbackToken != "" {
		options = append(options, xendit.WithCallbackToken(callbackToken))
	}
	return xendit.New(options...), nil
}

func (f *Factory) loadAccount(ctx context.Context, providerCode domain.ProviderCode, environment domain.Environment) (domain.ProviderAccount, error) {
	if f == nil || f.loader == nil {
		return domain.ProviderAccount{}, fmt.Errorf("provider account loader is required")
	}

	return f.loader.GetProviderAccount(ctx, providerCode, environment)
}

type midtransCredential struct {
	MerchantID string `json:"merchant_id"`
	ClientKey  string `json:"client_key"`
	ServerKey  string `json:"server_key"`
}

func midtransCredentialFromJSON(raw []byte) (midtransCredential, error) {
	var credential midtransCredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return midtransCredential{}, fmt.Errorf("read midtrans credential json: %w", err)
	}
	credential.MerchantID = strings.TrimSpace(credential.MerchantID)
	credential.ClientKey = strings.TrimSpace(credential.ClientKey)
	credential.ServerKey = strings.TrimSpace(credential.ServerKey)
	if credential.MerchantID == "" {
		return midtransCredential{}, fmt.Errorf("midtrans merchant id is not configured")
	}
	if credential.ClientKey == "" {
		return midtransCredential{}, fmt.Errorf("midtrans client key is not configured")
	}
	if credential.ServerKey == "" {
		return midtransCredential{}, fmt.Errorf("midtrans server key is not configured")
	}
	return credential, nil
}

func xenditSecretKeyFromJSON(raw []byte) (string, error) {
	var credential struct {
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		return "", fmt.Errorf("read xendit credential json: %w", err)
	}
	secretKey := strings.TrimSpace(credential.SecretKey)
	if secretKey == "" {
		return "", fmt.Errorf("xendit secret key is not configured")
	}
	return secretKey, nil
}

func xenditWebhookTokenFromConfig(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var config struct {
		WebhookToken string `json:"webhook_token"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("read xendit config json: %w", err)
	}
	return strings.TrimSpace(config.WebhookToken), nil
}
