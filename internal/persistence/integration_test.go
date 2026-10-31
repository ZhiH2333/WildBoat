package persistence_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/persistence"
)

func TestMigrateAndPing(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := persistence.MigrateUp(databaseURL); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	db, err := persistence.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	var value string
	err = db.Pool.QueryRow(ctx, `SELECT value FROM app_meta WHERE key = $1`, "schema_bootstrap").Scan(&value)
	if err != nil {
		t.Fatalf("query app_meta: %v", err)
	}
	if value != "phase1" {
		t.Fatalf("schema_bootstrap = %q, want phase1", value)
	}
}
