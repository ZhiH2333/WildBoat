package config

import (
	"fmt"
	"os"
	"strconv"
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

	TokenSigningKey string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	EmailMode       string
	AuthRateLimit   int
	AuthRateWindow  time.Duration
}

// Load reads configuration from environment variables and validates required fields.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:          strings.TrimSpace(firstNonEmpty(os.Getenv("APP_ENV"), "development")),
		HTTPAddr:        strings.TrimSpace(firstNonEmpty(os.Getenv("HTTP_ADDR"), "0.0.0.0:8080")),
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:        strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("LOG_LEVEL"), "info"))),
		EmailMode:       strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("EMAIL_MODE"), "none"))),
		TokenSigningKey: strings.TrimSpace(os.Getenv("TOKEN_SIGNING_KEY")),
	}

	shutdownRaw := strings.TrimSpace(firstNonEmpty(os.Getenv("SHUTDOWN_TIMEOUT"), "10s"))
	shutdownTimeout, err := time.ParseDuration(shutdownRaw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT %q: %w", shutdownRaw, err)
	}
	cfg.ShutdownTimeout = shutdownTimeout

	accessTTL, err := time.ParseDuration(strings.TrimSpace(firstNonEmpty(os.Getenv("ACCESS_TOKEN_TTL"), "15m")))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL: %w", err)
	}
	cfg.AccessTokenTTL = accessTTL

	refreshTTL, err := time.ParseDuration(strings.TrimSpace(firstNonEmpty(os.Getenv("REFRESH_TOKEN_TTL"), "720h")))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_TTL: %w", err)
	}
	cfg.RefreshTokenTTL = refreshTTL

	rateLimit, err := strconv.Atoi(strings.TrimSpace(firstNonEmpty(os.Getenv("AUTH_RATE_LIMIT"), "30")))
	if err != nil {
		return Config{}, fmt.Errorf("invalid AUTH_RATE_LIMIT: %w", err)
	}
	cfg.AuthRateLimit = rateLimit

	rateWindow, err := time.ParseDuration(strings.TrimSpace(firstNonEmpty(os.Getenv("AUTH_RATE_WINDOW"), "1m")))
	if err != nil {
		return Config{}, fmt.Errorf("invalid AUTH_RATE_WINDOW: %w", err)
	}
	cfg.AuthRateWindow = rateWindow

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
	if c.TokenSigningKey == "" {
		missing = append(missing, "TOKEN_SIGNING_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	if len(c.TokenSigningKey) < 32 {
		return fmt.Errorf("TOKEN_SIGNING_KEY must be at least 32 characters")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid LOG_LEVEL %q: must be one of debug, info, warn, error", c.LogLevel)
	}

	switch c.EmailMode {
	case "none", "dev":
	default:
		return fmt.Errorf("invalid EMAIL_MODE %q: must be one of none, dev", c.EmailMode)
	}
	if c.EmailMode == "dev" && !isNonProduction(c.AppEnv) {
		return fmt.Errorf("EMAIL_MODE=dev is only allowed when APP_ENV is development or test")
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be greater than zero")
	}
	if c.AccessTokenTTL <= 0 {
		return fmt.Errorf("ACCESS_TOKEN_TTL must be greater than zero")
	}
	if c.RefreshTokenTTL <= 0 {
		return fmt.Errorf("REFRESH_TOKEN_TTL must be greater than zero")
	}
	if c.AuthRateLimit <= 0 {
		return fmt.Errorf("AUTH_RATE_LIMIT must be greater than zero")
	}
	if c.AuthRateWindow <= 0 {
		return fmt.Errorf("AUTH_RATE_WINDOW must be greater than zero")
	}

	return nil
}

func isNonProduction(appEnv string) bool {
	switch strings.ToLower(appEnv) {
	case "development", "dev", "test", "local":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
