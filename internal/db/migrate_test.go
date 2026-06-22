package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
)

// TestRunMigrationsFreshDatabase verifies the full migration chain applies
// cleanly to an empty database — the fresh-install path. It guards against
// retroactive edits to applied migrations (e.g. a schema change baked into
// 001 that a later migration also applies, which passes on existing installs
// but fails on new ones).
//
// TEST_MIGRATE_DATABASE_URL must point at a DISPOSABLE database: the test
// drops every table in the public schema before running migrations. A plain
// Postgres install suffices — the schema is FTS-only, with no
// vector extension required.
func TestRunMigrationsFreshDatabase(t *testing.T) {
	dbURL := os.Getenv("TEST_MIGRATE_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_MIGRATE_DATABASE_URL not set")
	}

	ctx := context.Background()

	pool, err := dbpool.NewPool(ctx, dbURL, 5)
	if err != nil {
		t.Fatalf("connecting to migrate test DB: %v", err)
	}
	defer pool.Close()

	// Reset to an empty public schema before migrating.
	const dropAllTables = `
		DO $$
		DECLARE r RECORD;
		BEGIN
			FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
				EXECUTE format('DROP TABLE IF EXISTS public.%I CASCADE', r.tablename);
			END LOOP;
		END $$`
	if _, err := pool.Exec(ctx, dropAllTables); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		t.Fatalf("running migrations on fresh database: %v", err)
	}

	// After a full apply, nothing is pending — the daemon's boot check (used when
	// auto-migrate is off) must agree, or it would refuse to serve a current DB.
	pending, err := db.MigrationsPending(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("checking pending migrations: %v", err)
	}
	if pending {
		t.Error("MigrationsPending = true immediately after a full migration; want false")
	}

	// Spot-check the end-state schema: the migrations stand up
	// the index tables, with NO embedding column
	// and no leftover graph tables.
	assertColumn(ctx, t, pool, "notes", "superseded", true)
	assertColumn(ctx, t, pool, "chunks", "search_tsv", true)
	assertColumn(ctx, t, pool, "chunks", "embedding", false)
	assertTableAbsent(ctx, t, pool, "kg_nodes")

	// 003: notes-as-source-of-truth columns + the append-only version log.
	assertColumn(ctx, t, pool, "notes", "version", true)
	assertColumn(ctx, t, pool, "notes", "deleted", true)
	assertColumn(ctx, t, pool, "note_versions", "op", true)

	// 005: the RLS-exempt onboarding tables that route an IdP identity to a tenant.
	assertColumn(ctx, t, pool, "tenants", "label", true)
	assertColumn(ctx, t, pool, "identities", "tenant_id", true)

	// 009: retired-subsystem residue is dropped — the file-indexer sources table,
	// the StaticTokenAuth api_keys table, the never-populated links table, and the
	// vestigial notes.source_path column are all gone.
	assertTableAbsent(ctx, t, pool, "sources")
	assertTableAbsent(ctx, t, pool, "api_keys")
	assertTableAbsent(ctx, t, pool, "links")
	assertColumn(ctx, t, pool, "notes", "source_path", false)
}

func assertTableAbsent(ctx context.Context, t *testing.T, pool *dbpool.Pool, table string) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1)`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("checking table %s: %v", table, err)
	}
	if exists {
		t.Errorf("table %s should not exist after the migration", table)
	}
}

func assertColumn(ctx context.Context, t *testing.T, pool *dbpool.Pool, table, column string, want bool) {
	t.Helper()

	var got bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&got)
	if err != nil {
		t.Fatalf("checking column %s.%s: %v", table, column, err)
	}
	if got != want {
		t.Errorf("column %s.%s exists = %v, want %v", table, column, got, want)
	}
}
