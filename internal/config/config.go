package config

import (
	"os"
	"strings"
)

const (
	defaultDBPath      = "./rute-bayar.sqlite3"
	defaultWebhookAddr = ":8080"
)

type Config struct {
	Environment string
	DBPath      string
	WebhookAddr string
}

func Load() Config {
	fileEnv := readDotEnv(".env")
	return Config{
		Environment: envOrDefault(fileEnv, "RUTE_BAYAR_ENV", "sandbox"),
		DBPath:      envOrDefault(fileEnv, "RUTE_BAYAR_DB_PATH", defaultDBPath),
		WebhookAddr: envOrDefault(fileEnv, "RUTE_BAYAR_WEBHOOK_ADDR", defaultWebhookAddr),
	}
}

func envOrDefault(fileEnv map[string]string, key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fileEnv[key]
	}
	if value == "" {
		value = fallback
	}
	return value
}

func readDotEnv(path string) map[string]string {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			values[key] = value
		}
	}

	return values
}
