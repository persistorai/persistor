package index

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
)

const storeQueryTimeout = 30 * time.Second

// Store is tenant-scoped data access for the memory engine (notes, their
// append-only version history, and the chunk projection). Every operation runs
// in a transaction with app.tenant_id set so Postgres row-level security
// isolates tenants.
type Store struct {
	pool *dbpool.Pool
	log  *logrus.Logger
}

// NewStore returns an index store over the given pool.
func NewStore(pool *dbpool.Pool, log *logrus.Logger) *Store {
	return &Store{pool: pool, log: log}
}

// inTx runs fn in a read-write transaction with the tenant context set.
func (s *Store) inTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer s.rollback(ctx, tx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// inReadTx runs fn in a read-only transaction with the tenant context set.
func (s *Store) inReadTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}
	tx, err := s.pool.BeginReadOnly(ctx)
	if err != nil {
		return fmt.Errorf("beginning read transaction: %w", err)
	}
	defer s.rollback(ctx, tx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}
	return fn(tx)
}

// rollback aborts a transaction, ignoring the expected "already committed" case
// and logging any other failure. Safe to defer after a successful commit.
func (s *Store) rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		s.log.WithError(err).Warn("transaction rollback")
	}
}
