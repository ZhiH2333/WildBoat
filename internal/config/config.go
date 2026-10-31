package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	AppEnv          string
	HTTPAddr        string
	DatabaseURL     string
	LogLevel        string
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables and validates required fields.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:      strings.TrimSpace(firstNonEmpty(os.Getenv("APP_ENV"), "development")),
		HTTPAddr:    strings.TrimSpace(firstNonEmpty(os.Getenv("HTTP_ADDR"), "0.0.0.0:8080")),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:    strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("LOG_LEVEL"), "info"))),
	}

	shutdownRaw := strings.TrimSpace(firstNonEmpty(os.Getenv("SHUTDOWN_TIMEOUT"), "10s"))
	shutdownTimeout, err := time.ParseDuration(shutdownRaw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT %q: %w", shutdownRaw, err)
	}
	cfg.ShutdownTimeout = shutdownTimeout

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate fails fast on invalid required settings.
func (c Config) Validate() error {
	var missing []string
	if c.AppEnv == "" {
		missing = append(missing, "APP_ENV")
	}
	if c.HTTPAddr == "" {
		missing = append(missing, "HTTP_ADDR")
	}
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid LOG_LEVEL %q: must be one of debug, info, warn, error", c.LogLevel)
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be greater than zero")
	}

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
