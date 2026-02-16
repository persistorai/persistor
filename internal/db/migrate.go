// Migration runner using goose (github.com/pressly/goose/v3).
//
// Choice rationale: goose was chosen over golang-migrate for this project because:
//   - Simpler API with fewer moving parts (single provider, no separate source/database drivers)
//   - Up/down migrations live in the same file (-- +goose Up / -- +goose Down)
//   - Native embed.FS support without adapter wrappers
//   - Programmatic usage is straightforward (goose.NewProvider)
//
// Migration files live in internal/db/migrations/ and are embedded via //go:embed.
// On startup, RunMigrations applies all pending migrations automatically.
//
// Compatibility: goose uses its own version table (goose_db_version). The old
// hand-rolled schema_migrations table is left in place (harmless) but no longer used.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx as database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
)

// RunMigrations applies all pending migrations from the provided filesystem.
// The fsys should contain goose-annotated SQL files (e.g. "001_initial.sql").
func RunMigrations(ctx context.Context, pool *dbpool.Pool, log *logrus.Logger, fsys fs.FS) error {
	// goose requires a *sql.DB. Acquire a raw connection from the pgx pool
	// and wrap it via the pgx stdlib driver.
	connStr := pool.ConnString()

	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("opening sql.DB for migrations: %w", err)
	}
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		return fmt.Errorf("creating goose provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", r.Source.Version, r.Source.Path, r.Error)
		}

		log.WithFields(logrus.Fields{
			"version":  r.Source.Version,
			"file":     r.Source.Path,
			"duration": r.Duration,
		}).Info("migration applied")
	}

	if len(results) == 0 {
		log.Debug("all migrations already applied")
	}

	return nil
}
