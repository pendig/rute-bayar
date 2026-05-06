package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

type boolFlag struct {
	value bool
	set   bool
}

func (f *boolFlag) Set(value string) error {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = parsed
	f.set = true
	return nil
}

func (f *boolFlag) String() string {
	return strconv.FormatBool(f.value)
}

type stringMapFlag struct {
	values map[string]string
	set    bool
}

func (f *stringMapFlag) Set(value string) error {
	key, mappedValue, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("invalid key-value pair %q, expected key=value", value)
	}
	key = strings.TrimSpace(key)
	mappedValue = strings.TrimSpace(mappedValue)
	if key == "" {
		return fmt.Errorf("invalid key-value pair %q, key cannot be empty", value)
	}

	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = mappedValue
	f.set = true
	return nil
}

func (f *stringMapFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	return "key=value"
}

type stringSliceMapFlag struct {
	values map[string][]string
	set    bool
}

func (f *stringSliceMapFlag) Set(value string) error {
	key, mappedValue, found := strings.Cut(value, "=")
	if !found {
		return fmt.Errorf("invalid key-value pair %q, expected key=value", value)
	}
	key = strings.TrimSpace(key)
	mappedValue = strings.TrimSpace(mappedValue)
	if key == "" {
		return fmt.Errorf("invalid key-value pair %q, key cannot be empty", value)
	}

	if f.values == nil {
		f.values = map[string][]string{}
	}
	f.values[key] = append(f.values[key], mappedValue)
	f.set = true
	return nil
}

func (f *stringSliceMapFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	return "key=value"
}

type intFlag struct {
	value int
	set   bool
}

func (f *intFlag) Set(value string) error {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = v
	f.set = true
	return nil
}

func (f *intFlag) String() string {
	return strconv.Itoa(f.value)
}

type durationFlag struct {
	value time.Duration
	set   bool
}

func (f *durationFlag) Set(value string) error {
	v, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.value = v
	f.set = true
	return nil
}

func (f *durationFlag) String() string {
	return f.value.String()
}

func allProviders() []domain.ProviderCode {
	return []domain.ProviderCode{domain.ProviderMidtrans, domain.ProviderXendit}
}

func parseProvider(value string) (domain.ProviderCode, error) {
	provider := strings.ToLower(strings.TrimSpace(value))
	for _, supportedProvider := range allProviders() {
		if provider == string(supportedProvider) {
			return supportedProvider, nil
		}
	}

	valid := make([]string, 0, len(allProviders()))
	for _, supportedProvider := range allProviders() {
		valid = append(valid, string(supportedProvider))
	}
	return "", fmt.Errorf("provider must be one of %q", strings.Join(valid, "\", \""))
}

func convertMapToHeaders(values map[string]string) http.Header {
	headers := http.Header{}
	for key, value := range values {
		headers.Add(key, value)
	}
	return headers
}

func convertSliceMapToHeaders(values map[string][]string) http.Header {
	headers := http.Header{}
	for key, list := range values {
		for _, value := range list {
			headers.Add(key, value)
		}
	}
	return headers
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

func copyStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return map[string][]string{}
	}
	copied := make(map[string][]string, len(values))
	for key, list := range values {
		copiedValues := make([]string, len(list))
		copy(copiedValues, list)
		copied[key] = copiedValues
	}
	return copied
}

func validateEnvironment(value string) error {
	switch domain.Environment(value) {
	case domain.EnvironmentSandbox, domain.EnvironmentProduction:
		return nil
	default:
		return fmt.Errorf("environment must be %q or %q", domain.EnvironmentSandbox, domain.EnvironmentProduction)
	}
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func secretKeyFromCredential(raw []byte) (string, error) {
	var credential struct {
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		return "", fmt.Errorf("read credential json: %w", err)
	}
	secretKey := strings.TrimSpace(credential.SecretKey)
	if secretKey == "" {
		return "", fmt.Errorf("xendit secret key is not configured")
	}
	return secretKey, nil
}

func secretKeyFromCredentialFromStore(store *sqlite.Store, ctx context.Context, providerCode domain.ProviderCode, environment domain.Environment) (string, error) {
	account, err := store.GetProviderAccount(ctx, providerCode, environment)
	if err != nil {
		return "", err
	}
	return secretKeyFromCredential(account.CredentialJSON)
}

func isXenditPayMethodSupported(method string) bool {
	return strings.EqualFold(strings.TrimSpace(method), "payment_link") ||
		strings.EqualFold(strings.TrimSpace(method), "payment-link") ||
		strings.EqualFold(strings.TrimSpace(method), "paymentlink") ||
		strings.EqualFold(strings.TrimSpace(method), "")
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
