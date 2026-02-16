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

// Pool wraps a pgxpool.Pool with health check capabilities.
// The underlying pool is unexported to prevent callers from bypassing
// the withTimeout pattern used by Repository methods.
type Pool struct {
	pool *pgxpool.Pool
}

// NewPool creates a new PostgreSQL connection pool with sensible defaults.
func NewPool(ctx context.Context, databaseURL string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"

	cfg.MaxConns = 21 // 20 for queries + 1 for LISTEN/NOTIFY bridge
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

	return &Pool{pool: pool}, nil
}

// Acquire returns a connection from the pool.
func (p *Pool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	return p.pool.Acquire(ctx)
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

// BeginTx starts a transaction with the given options.
func (p *Pool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) { //nolint:gocritic // matching pgxpool.Pool signature.
	return p.pool.BeginTx(ctx, txOptions)
}

// Ping verifies the pool can reach the database.
func (p *Pool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// HealthCheck verifies database connectivity by executing a simple query.
func (p *Pool) HealthCheck(ctx context.Context) error {
	var result int

	err := p.pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("health check query: %w", err)
	}

	return nil
}

// ConnString returns the connection string used to create the pool.
func (p *Pool) ConnString() string {
	return p.pool.Config().ConnString()
}

// Close closes the connection pool.
func (p *Pool) Close() {
	p.pool.Close()
}
