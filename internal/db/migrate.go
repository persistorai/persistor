// Migration runner using goose (github.com/pressly/goose/v3).
//
// Choice rationale: goose was chosen over golang-migrate for this project because:
// - Simpler API with fewer moving parts (single provider, no separate source/database drivers)
// - Up/down migrations live in the same file (-- +goose Up / -- +goose Down)
// - Native embed.FS support without adapter wrappers
// - Programmatic usage is straightforward (goose.NewProvider)
//
// Migration files live in internal/db/migrations/ and are embedded via //go:embed.
// RunMigrations applies all pending migrations as the schema-owning role (the
// daemon at boot when PERSISTOR_AUTO_MIGRATE is on, or `persistor migrate`).
// MigrationsPending is the read-only check the daemon uses to fail closed when
// auto-migrate is off and the schema is stale.
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

// openProvider builds a goose provider over a database/sql connection wrapped
// around the pgx pool's connection string. The caller must close the returned
// *sql.DB. The all-zeros placeholder tenant only satisfies RLS policy evaluation
// during DDL (which isn't subject to RLS); it is session-scoped (is_local=false)
// because goose runs each migration across its own statements, not one wrapped
// transaction, so the setting must outlive any single tx.
func openProvider(ctx context.Context, pool *dbpool.Pool, fsys fs.FS) (*goose.Provider, *sql.DB, error) {
	sqlDB, err := sql.Open("pgx", pool.ConnString())
	if err != nil {
		return nil, nil, fmt.Errorf("opening sql.DB for migrations: %w", err)
	}

	if _, err := sqlDB.ExecContext(ctx, "SELECT set_config('app.tenant_id', '00000000-0000-0000-0000-000000000000', false)"); err != nil {
		sqlDB.Close()
		return nil, nil, fmt.Errorf("initializing migration tenant setting: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		sqlDB.Close()
		return nil, nil, fmt.Errorf("creating goose provider: %w", err)
	}
	return provider, sqlDB, nil
}

// MigrationsPending reports whether any embedded migration has not yet been
// applied to the database. The daemon uses it to fail closed at boot when
// auto-migrate is off (the production posture): a non-owner app role cannot run
// DDL, so it must refuse to serve against a schema that an operator has not yet
// migrated with `persistor migrate`, rather than erroring on the first query.
func MigrationsPending(ctx context.Context, pool *dbpool.Pool, fsys fs.FS) (bool, error) {
	provider, sqlDB, err := openProvider(ctx, pool, fsys)
	if err != nil {
		return false, err
	}
	defer sqlDB.Close()

	pending, err := provider.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("checking pending migrations: %w", err)
	}
	return pending, nil
}

// RunMigrations applies all pending migrations from the provided filesystem.
// The fsys should contain goose-annotated SQL files (e.g. "001_initial.sql").
// It must run as the schema-owning migrator role; a least-privilege app role
// cannot execute the DDL.
func RunMigrations(ctx context.Context, pool *dbpool.Pool, log *logrus.Logger, fsys fs.FS) error {
	provider, sqlDB, err := openProvider(ctx, pool, fsys)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

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
