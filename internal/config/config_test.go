package config

import (
	"testing"
	"time"
)

func TestValidateRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv:                    "development",
		HTTPAddr:                  "127.0.0.1:8080",
		DatabaseURL:               "",
		LogLevel:                  "info",
		ShutdownTimeout:           5 * time.Second,
		TokenSigningKey:           "0123456789abcdef0123456789abcdef",
		AccessTokenTTL:            15 * time.Minute,
		RefreshTokenTTL:           24 * time.Hour,
		EmailMode:                 "none",
		AuthRateLimit:             30,
		AuthRateWindow:            time.Minute,
		DedicatedServerCredential: "local-ds-credential-32chars-min!!",
		GameTicketTTL:             60 * time.Second,
		DSHeartbeatTTL:            30 * time.Second,
		GameInstanceCapacity:      4,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing DATABASE_URL")
	}
}

func TestValidateRejectsInvalidLogLevel(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv:                    "development",
		HTTPAddr:                  "127.0.0.1:8080",
		DatabaseURL:               "postgres://wildboat:secret@localhost:5432/wildboat?sslmode=disable",
		LogLevel:                  "verbose",
		ShutdownTimeout:           5 * time.Second,
		TokenSigningKey:           "0123456789abcdef0123456789abcdef",
		AccessTokenTTL:            15 * time.Minute,
		RefreshTokenTTL:           24 * time.Hour,
		EmailMode:                 "none",
		AuthRateLimit:             30,
		AuthRateWindow:            time.Minute,
		DedicatedServerCredential: "local-ds-credential-32chars-min!!",
		GameTicketTTL:             60 * time.Second,
		DSHeartbeatTTL:            30 * time.Second,
		GameInstanceCapacity:      4,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid LOG_LEVEL")
	}
}

func TestValidateRejectsDevEmailInProduction(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.AppEnv = "production"
	cfg.EmailMode = "dev"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for EMAIL_MODE=dev in production")
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("DATABASE_URL", "postgres://wildboat:secret@localhost:5432/wildboat?sslmode=disable")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("TOKEN_SIGNING_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ACCESS_TOKEN_TTL", "10m")
	t.Setenv("REFRESH_TOKEN_TTL", "48h")
	t.Setenv("EMAIL_MODE", "dev")
	t.Setenv("AUTH_RATE_LIMIT", "12")
	t.Setenv("AUTH_RATE_WINDOW", "30s")
	t.Setenv("DEDICATED_SERVER_CREDENTIAL", "local-ds-credential-32chars-min!!")
	t.Setenv("GAME_TICKET_TTL", "45s")
	t.Setenv("DS_HEARTBEAT_TTL", "20s")
	t.Setenv("GAME_INSTANCE_CAPACITY", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.AppEnv != "test" {
		t.Fatalf("AppEnv = %q, want test", cfg.AppEnv)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddr = %q, want 127.0.0.1:9090", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 3s", cfg.ShutdownTimeout)
	}
	if cfg.AccessTokenTTL != 10*time.Minute {
		t.Fatalf("AccessTokenTTL = %v, want 10m", cfg.AccessTokenTTL)
	}
	if cfg.EmailMode != "dev" {
		t.Fatalf("EmailMode = %q, want dev", cfg.EmailMode)
	}
	if cfg.AuthRateLimit != 12 {
		t.Fatalf("AuthRateLimit = %d, want 12", cfg.AuthRateLimit)
	}
	if cfg.GameTicketTTL != 45*time.Second {
		t.Fatalf("GameTicketTTL = %v, want 45s", cfg.GameTicketTTL)
	}
	if cfg.GameInstanceCapacity != 2 {
		t.Fatalf("GameInstanceCapacity = %d, want 2", cfg.GameInstanceCapacity)
	}
}

func validConfig() Config {
	return Config{
		AppEnv:                    "development",
		HTTPAddr:                  "127.0.0.1:8080",
		DatabaseURL:               "postgres://wildboat:secret@localhost:5432/wildboat?sslmode=disable",
		LogLevel:                  "info",
		ShutdownTimeout:           10 * time.Second,
		TokenSigningKey:           "0123456789abcdef0123456789abcdef",
		AccessTokenTTL:            15 * time.Minute,
		RefreshTokenTTL:           720 * time.Hour,
		EmailMode:                 "dev",
		AuthRateLimit:             30,
		AuthRateWindow:            time.Minute,
		DedicatedServerCredential: "local-ds-credential-32chars-min!!",
		GameTicketTTL:             60 * time.Second,
		DSHeartbeatTTL:            30 * time.Second,
		GameInstanceCapacity:      4,
	}
}
