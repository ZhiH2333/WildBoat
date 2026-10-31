package persistence

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/ZhiH2333/WildBoat/migrations"
)

// MigrateUp applies all pending SQL migrations.
func MigrateUp(databaseURL string) error {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, toMigrateURL(databaseURL))
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// toMigrateURL rewrites postgres:// to pgx5:// for the migrate pgx/v5 driver.
func toMigrateURL(databaseURL string) string {
	switch {
	case len(databaseURL) >= 13 && databaseURL[:13] == "postgresql://":
		return "pgx5://" + databaseURL[13:]
	case len(databaseURL) >= 11 && databaseURL[:11] == "postgres://":
		return "pgx5://" + databaseURL[11:]
	default:
		return databaseURL
	}
}
