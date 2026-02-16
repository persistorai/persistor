// Package store provides focused, single-concern data access stores
// for the Persistor knowledge graph.
//
// Each store owns one domain (nodes, edges, search, graph, etc.) and
// embeds shared helpers (Pool, crypto, logger) via the Base struct.
// Stores never import each other — shared logic lives in this file
// or in dedicated helper files (encrypt.go, notify.go).
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/crypto"
	"github.com/persistorai/persistor/internal/dbpool"
)

const defaultQueryTimeout = 30 * time.Second

// Base contains shared dependencies for all stores.
// Embed this in each store struct.
type Base struct {
	Pool   *dbpool.Pool
	Log    *logrus.Logger
	Crypto *crypto.Service
}

// withTimeout creates a context with the default query timeout.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultQueryTimeout)
}

// setTenant sets the tenant context for RLS policies within a transaction.
func setTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant ID format: %w", err)
	}

	_, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID)
	if err != nil {
		return fmt.Errorf("setting tenant context: %w", err)
	}

	return nil
}

// beginTx starts a read-write transaction and sets the tenant context.
func (b *Base) beginTx(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := b.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}

	if err := setTenant(ctx, tx, tenantID); err != nil {
		tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on setup failure.

		return nil, err
	}

	return tx, nil
}

// beginReadTx starts a read-only transaction and sets the tenant context.
func (b *Base) beginReadTx(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := b.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("beginning read transaction: %w", err)
	}

	if err := setTenant(ctx, tx, tenantID); err != nil {
		tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on setup failure.

		return nil, err
	}

	return tx, nil
}

// notify sends a pg_notify on the kg_changes channel (best-effort, post-commit).
func (b *Base) notify(table, op, tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload, _ := json.Marshal(map[string]any{ //nolint:errcheck // static keys, cannot fail.
		"table":     table,
		"op":        op,
		"count":     1,
		"tenant_id": tenantID,
	})
	if _, err := b.Pool.Exec(ctx, "SELECT pg_notify('kg_changes', $1)", string(payload)); err != nil {
		b.Log.WithError(err).Warn("failed to send " + op + " " + table + " notification")
	}
}

// GetTenantByAPIKey looks up a tenant ID by API key hash.
func (b *Base) GetTenantByAPIKey(ctx context.Context, apiKey string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	hash := sha256.Sum256([]byte(apiKey))
	apiKeyHash := hex.EncodeToString(hash[:])

	var tenantID string

	err := b.Pool.QueryRow(ctx, "SELECT id FROM tenants WHERE api_key_hash = $1", apiKeyHash).Scan(&tenantID)
	if err != nil {
		return "", fmt.Errorf("looking up tenant by API key: %w", err)
	}

	return tenantID, nil
}
