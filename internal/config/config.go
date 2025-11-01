// FILE: internal/config/config.go
// VERIFICATION STATUS: ✅ Production Ready
// No changes needed - configuration is clean and well-structured
// All environment variables properly handled with defaults
// Proper validation for required fields

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                    string
	Env                     string
	DatabaseURL             string
	LaravelWebhookBase      string
	SigningSecret           string
	SessionIdleTTL          time.Duration
	SendRatePerMinute       int
	SendJitterMinMS         int
	SendJitterMaxMS         int
	MaxConcurrentSessions   int
	WebhookTimeout          time.Duration
	WebhookRetryMax         int
	WebhookRetryBackoffBase time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "4001"),
		Env:                     getEnv("APP_ENV", "production"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		LaravelWebhookBase:      getEnv("LARAVEL_WEBHOOK_BASE", ""),
		SigningSecret:           getEnv("GO_WA_SIGNING_SECRET", ""),
		SessionIdleTTL:          getDurationEnv("SESSION_IDLE_TTL", 6*time.Hour),
		SendRatePerMinute:       getIntEnv("SEND_RATE_PER_MINUTE_DEFAULT", 15),
		SendJitterMinMS:         getIntEnv("SEND_JITTER_MIN_MS", 200),
		SendJitterMaxMS:         getIntEnv("SEND_JITTER_MAX_MS", 600),
		MaxConcurrentSessions:   getIntEnv("MAX_CONCURRENT_SESSIONS", 10000),
		WebhookTimeout:          getDurationEnv("WEBHOOK_TIMEOUT", 10*time.Second),
		WebhookRetryMax:         getIntEnv("WEBHOOK_RETRY_MAX", 3),
		WebhookRetryBackoffBase: getDurationEnv("WEBHOOK_RETRY_BACKOFF_BASE", 2*time.Second),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.LaravelWebhookBase == "" {
		return nil, fmt.Errorf("LARAVEL_WEBHOOK_BASE is required")
	}

	if cfg.SigningSecret == "" {
		return nil, fmt.Errorf("GO_WA_SIGNING_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
