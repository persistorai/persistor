// Package main provides a standalone migration script that reads a knowledge graph
// from SQLite and writes it to PostgreSQL for the Persistor.
//
// Usage:
//
//	SQLITE_PATH=/path/to/sqlite DATABASE_URL=postgres://... go run scripts/migrate-from-sqlite.go
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	_ "modernc.org/sqlite"
)

// config holds environment-driven migration settings.
type config struct {
	SQLitePath  string
	DatabaseURL string
	TenantID    string
	TenantName  string
	DryRun      bool
	enc         *encryptor
}

// skippedEdge records an edge that was skipped during migration.
type skippedEdge struct {
	Source string
	Target string
	Reason string
}

// report holds the final migration summary.
type report struct {
	Source        string
	Target        string
	TenantName    string
	TenantID      string
	NodesRead     int
	NodesInserted int
	NodesVerified int
	EdgesRead     int
	EdgesInserted int
	EdgesSkipped  int
	EdgesVerified int
	SkippedEdges  []skippedEdge
	SpotChecks    []string
	Duration      time.Duration
	DryRun        bool
	Err           error
}

func main() {
	cfg := loadConfig()
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	// Initialize encryption — required for property security.
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		slog.Error("ENCRYPTION_KEY is required (hex-encoded 32-byte AES-256 key)")
		os.Exit(1)
	}

	enc, err := newEncryptor(encKey)
	if err != nil {
		slog.Error("failed to initialize encryption", "error", err)
		os.Exit(1)
	}

	cfg.enc = enc
	slog.Info("property encryption enabled")

	slog.Info("starting migration",
		"sqlite", cfg.SQLitePath,
		"tenant", cfg.TenantName,
		"dry_run", cfg.DryRun,
	)

	start := time.Now()
	r, err := runMigration(context.Background(), cfg)
	r.Duration = time.Since(start)
	if err != nil {
		r.Err = err
		slog.Error("migration failed", "error", err)
	}
	printReport(&r)
	if err != nil {
		os.Exit(1)
	}
}

// loadConfig reads configuration from environment variables.
func loadConfig() config {
	c := config{
		SQLitePath:  envOr("SQLITE_PATH", "memory/main.sqlite"),
		DatabaseURL: envOr("DATABASE_URL", ""),
		TenantName:  "persistor-default",
		DryRun:      os.Getenv("DRY_RUN") == "true" || os.Getenv("DRY_RUN") == "1",
	}
	if tid := os.Getenv("TENANT_ID"); tid != "" {
		c.TenantID = tid
	} else {
		c.TenantID = deterministicUUID(c.TenantName)
	}
	return c
}

// deterministicUUID generates a UUID v5-like deterministic UUID from a name
// using SHA-256 and formatting as a UUID string.
func deterministicUUID(name string) string {
	h := sha256.Sum256([]byte("persistor:" + name))
	// Set version 5 and variant bits.
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// ensureTenant creates the tenant row if it doesn't already exist.
func ensureTenant(ctx context.Context, tx pgx.Tx, tenantID, name string) error {
	slog.Info("ensuring tenant exists", "id", tenantID, "name", name)
	hash := sha256.Sum256([]byte("migration-" + tenantID))
	apiKeyHash := fmt.Sprintf("%x", hash)
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, api_key_hash, plan)
		 VALUES ($1, $2, $3, 'free')
		 ON CONFLICT (id) DO NOTHING`,
		tenantID, name, apiKeyHash)
	return err
}

// runMigration executes the full migration pipeline.
//
//nolint:funlen // Migration pipeline is sequential; splitting would hurt readability.
func runMigration(ctx context.Context, cfg config) (report, error) {
	r := report{
		Source:     cfg.SQLitePath,
		Target:     sanitizeURL(cfg.DatabaseURL),
		TenantName: cfg.TenantName,
		TenantID:   cfg.TenantID,
		DryRun:     cfg.DryRun,
	}

	// Open SQLite (read-only).
	lite, err := sql.Open("sqlite", cfg.SQLitePath+"?mode=ro")
	if err != nil {
		return r, fmt.Errorf("open sqlite: %w", err)
	}
	defer lite.Close()

	// Read nodes and edges from SQLite.
	nodes, err := readNodes(ctx, lite)
	if err != nil {
		return r, fmt.Errorf("read nodes: %w", err)
	}
	r.NodesRead = len(nodes)
	slog.Info("read nodes from sqlite", "count", r.NodesRead)

	edges, err := readEdges(ctx, lite)
	if err != nil {
		return r, fmt.Errorf("read edges: %w", err)
	}
	r.EdgesRead = len(edges)
	slog.Info("read edges from sqlite", "count", r.EdgesRead)

	if cfg.DryRun {
		slog.Info("dry run — skipping PostgreSQL writes")
		r.NodesInserted = r.NodesRead
		r.EdgesInserted = r.EdgesRead
		return r, nil
	}

	// Connect to PostgreSQL and run in a transaction.
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return r, fmt.Errorf("connect postgres: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return r, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	if err := ensureTenant(ctx, tx, cfg.TenantID, cfg.TenantName); err != nil {
		return r, fmt.Errorf("ensure tenant: %w", err)
	}

	if err := insertNodes(ctx, tx, nodes, cfg.TenantID, cfg.enc); err != nil {
		return r, fmt.Errorf("insert nodes: %w", err)
	}
	r.NodesInserted = len(nodes)
	slog.Info("inserted nodes", "count", r.NodesInserted)

	inserted, skipped := insertEdges(ctx, tx, edges, cfg.TenantID, buildNodeSet(nodes), cfg.enc)
	r.EdgesInserted = inserted
	r.EdgesSkipped = len(skipped)
	r.SkippedEdges = skipped
	slog.Info("inserted edges", "count", r.EdgesInserted, "skipped", r.EdgesSkipped)

	// Verify counts.
	r.NodesVerified, err = countRows(ctx, tx, "kg_nodes", cfg.TenantID)
	if err != nil {
		return r, fmt.Errorf("verify node count: %w", err)
	}
	r.EdgesVerified, err = countRows(ctx, tx, "kg_edges", cfg.TenantID)
	if err != nil {
		return r, fmt.Errorf("verify edge count: %w", err)
	}

	// Spot-check random nodes.
	r.SpotChecks, err = spotCheck(ctx, tx, lite, nodes, cfg.TenantID)
	if err != nil {
		return r, fmt.Errorf("spot check: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return r, fmt.Errorf("commit: %w", err)
	}
	slog.Info("transaction committed")
	return r, nil
}
