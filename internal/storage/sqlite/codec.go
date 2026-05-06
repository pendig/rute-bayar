package sqlite

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/forwarding"
)

func headersFromJSON(raw string) http.Header {
	if raw == "" || raw == "{}" {
		return http.Header{}
	}

	rawEntries := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(raw), &rawEntries); err != nil {
		return http.Header{}
	}

	headers := http.Header{}
	for key, rawValue := range rawEntries {
		var multiValues []string
		if err := json.Unmarshal(rawValue, &multiValues); err == nil {
			for _, value := range multiValues {
				headers.Add(key, value)
			}
			continue
		}

		var singleValue string
		if err := json.Unmarshal(rawValue, &singleValue); err == nil {
			headers.Set(key, singleValue)
		}
	}

	if len(headers) == 0 {
		return http.Header{}
	}
	return headers
}

func eventFilterFromJSON(raw string) map[string]string {
	if raw == "" || raw == "{}" {
		return map[string]string{}
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return map[string]string{}
	}

	filter := make(map[string]string, len(values))
	for key, value := range values {
		filter[key] = value
	}
	return filter
}

func headersToJSON(headers http.Header) string {
	values := make(map[string][]string, len(headers))
	for key, headerValues := range headers {
		if len(headerValues) > 0 {
			values[key] = headerValues
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func eventFilterToJSON(filter map[string]string) string {
	if len(filter) == 0 {
		return "{}"
	}
	values := make(map[string]string, len(filter))
	for key, value := range filter {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		values[key] = value
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func retryPolicyFromJSON(raw string) forwarding.RetryPolicy {
	if raw == "" || raw == "{}" {
		return forwarding.RetryPolicy{}
	}
	var dto struct {
		MaxAttempts int `json:"max_attempts"`
		TimeoutSec  int `json:"timeout_sec"`
		BackoffSec  int `json:"backoff_sec"`
	}
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return forwarding.RetryPolicy{}
	}
	return forwarding.RetryPolicy{
		MaxAttempts: dto.MaxAttempts,
		Timeout:     time.Duration(dto.TimeoutSec) * time.Second,
		Backoff:     time.Duration(dto.BackoffSec) * time.Second,
	}
}

func retryPolicyToJSON(policy forwarding.RetryPolicy) string {
	dto := struct {
		MaxAttempts int `json:"max_attempts"`
		TimeoutSec  int `json:"timeout_sec"`
		BackoffSec  int `json:"backoff_sec"`
	}{
		MaxAttempts: policy.MaxAttempts,
		TimeoutSec:  int(policy.Timeout / time.Second),
		BackoffSec:  int(policy.Backoff / time.Second),
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
