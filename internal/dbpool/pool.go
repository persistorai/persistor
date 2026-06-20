// Package dbpool provides PostgreSQL connection pool management.
package dbpool

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgxpool.Pool. The underlying pool is unexported so callers go
// through Begin/BeginReadOnly and the per-operation timeout pattern in
// internal/index rather than issuing raw queries.
type Pool struct {
	pool *pgxpool.Pool
}

// NewPool creates a new PostgreSQL connection pool with sensible defaults.
// maxConns sets the pool size; MinConns is held at 2 for warm connections.
func NewPool(ctx context.Context, databaseURL string, maxConns int32) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"

	cfg.MaxConns = maxConns
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := assertRLSEnforceable(ctx, pool); err != nil {
		pool.Close()

		return nil, err
	}

	return &Pool{pool: pool}, nil
}

// assertRLSEnforceable fails closed when the connection's role would bypass
// row-level security. RLS is the only thing isolating tenants, and a SUPERUSER
// or BYPASSRLS role silently ignores it (USING/WITH CHECK policies are never
// applied), so refuse to start rather than serve tenant data unprotected.
func assertRLSEnforceable(ctx context.Context, pool *pgxpool.Pool) error {
	var role string
	var bypasses bool
	err := pool.QueryRow(ctx,
		`SELECT current_user,
		        current_setting('is_superuser')::boolean
		          OR EXISTS (SELECT 1 FROM pg_roles
		                      WHERE rolname = current_user AND rolbypassrls)`).Scan(&role, &bypasses)
	if err != nil {
		return fmt.Errorf("checking RLS enforceability: %w", err)
	}
	if bypasses {
		return fmt.Errorf("database role %q bypasses row-level security (SUPERUSER or BYPASSRLS); "+
			"connect as a NOSUPERUSER NOBYPASSRLS role so tenant isolation is enforced", role)
	}
	return nil
}

// Exec executes a query that doesn't return rows.
func (p *Pool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return p.pool.Exec(ctx, sql, arguments...)
}

// Query executes a query that returns rows.
func (p *Pool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns at most one row.
func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.pool.QueryRow(ctx, sql, args...)
}

// Begin starts a transaction.
func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.pool.Begin(ctx)
}

// BeginReadOnly starts a read-only transaction.
func (p *Pool) BeginReadOnly(ctx context.Context) (pgx.Tx, error) {
	return p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
}

// Ping verifies a usable connection to the database, for readiness probes.
func (p *Pool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// ConnString returns the connection string used to create the pool.
func (p *Pool) ConnString() string {
	return p.pool.Config().ConnString()
}

// Close closes the connection pool.
func (p *Pool) Close() {
	p.pool.Close()
}
