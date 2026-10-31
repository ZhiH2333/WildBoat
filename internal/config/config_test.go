package config

import (
	"testing"
	"time"
)

func TestValidateRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv:          "development",
		HTTPAddr:        "127.0.0.1:8080",
		DatabaseURL:     "",
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing DATABASE_URL")
	}
}

func TestValidateRejectsInvalidLogLevel(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv:          "development",
		HTTPAddr:        "127.0.0.1:8080",
		DatabaseURL:     "postgres://wildboat:secret@localhost:5432/wildboat?sslmode=disable",
		LogLevel:        "verbose",
		ShutdownTimeout: 5 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid LOG_LEVEL")
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv:          "development",
		HTTPAddr:        "127.0.0.1:8080",
		DatabaseURL:     "postgres://wildboat:secret@localhost:5432/wildboat?sslmode=disable",
		LogLevel:        "info",
		ShutdownTimeout: 10 * time.Second,
	}
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
}
