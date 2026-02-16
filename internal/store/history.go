package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// HistoryStore handles property change history operations.
type HistoryStore struct {
	Base
}

// NewHistoryStore creates a new HistoryStore.
func NewHistoryStore(base Base) *HistoryStore {
	return &HistoryStore{Base: base}
}

// propertyDiff represents a single property value change.
type propertyDiff struct {
	key      string
	oldValue json.RawMessage
	newValue json.RawMessage
}

// diffProperties computes changed, added, and removed keys between two property maps.
func diffProperties(oldProps, newProps map[string]any) ([]propertyDiff, error) {
	var diffs []propertyDiff

	for k, newVal := range newProps {
		newJSON, err := json.Marshal(newVal)
		if err != nil {
			return nil, fmt.Errorf("marshalling new value for %s: %w", k, err)
		}

		oldVal, existed := oldProps[k]
		if !existed {
			diffs = append(diffs, propertyDiff{key: k, oldValue: nil, newValue: newJSON})

			continue
		}

		oldJSON, err := json.Marshal(oldVal)
		if err != nil {
			return nil, fmt.Errorf("marshalling old value for %s: %w", k, err)
		}

		if !bytes.Equal(oldJSON, newJSON) {
			diffs = append(diffs, propertyDiff{key: k, oldValue: oldJSON, newValue: newJSON})
		}
	}

	for k, oldVal := range oldProps {
		if _, exists := newProps[k]; !exists {
			oldJSON, err := json.Marshal(oldVal)
			if err != nil {
				return nil, fmt.Errorf("marshalling removed value for %s: %w", k, err)
			}

			diffs = append(diffs, propertyDiff{key: k, oldValue: oldJSON, newValue: nil})
		}
	}

	return diffs, nil
}

// RecordPropertyChanges diffs oldProps and newProps, inserting a history row
// for each changed key. Package-level so NodeStore can call it within its transaction.
func RecordPropertyChanges(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, nodeID string,
	oldProps, newProps map[string]any,
	reason string,
) error {
	changes, err := diffProperties(oldProps, newProps)
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		return nil
	}

	valueParts := make([]string, 0, len(changes))
	args := make([]any, 0, len(changes)*6)

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	for i, c := range changes {
		base := i*6 + 1
		valueParts = append(valueParts, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d)",
			base, base+1, base+2, base+3, base+4, base+5,
		))
		args = append(args, tenantID, nodeID, c.key, c.oldValue, c.newValue, reasonPtr)
	}

	sql := `INSERT INTO kg_property_history (tenant_id, node_id, property_key, old_value, new_value, reason)
		VALUES ` + strings.Join(valueParts, ", ")

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("inserting property history: %w", err)
	}

	return nil
}

// fetchNodeProperties loads and decrypts properties for a single node within a transaction.
// Package-level so NodeStore.UpdateNode can call it.
func fetchNodeProperties(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, nodeID string,
	b *Base,
) (map[string]any, error) {
	var propsBytes []byte

	err := tx.QueryRow(ctx,
		`SELECT properties FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
		nodeID,
	).Scan(&propsBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNodeNotFound
		}

		return nil, fmt.Errorf("fetching node properties: %w", err)
	}

	props, err := b.decryptPropertiesRaw(ctx, tenantID, propsBytes)
	if err != nil {
		return nil, fmt.Errorf("decrypting node properties: %w", err)
	}

	return props, nil
}

// GetPropertyHistory returns property change history for a node with optional
// key filter and has_more pagination.
func (s *HistoryStore) GetPropertyHistory(
	ctx context.Context,
	tenantID, nodeID string,
	propertyKey string,
	limit, offset int,
) ([]models.PropertyChange, bool, error) {
	if limit <= 0 {
		limit = 50
	}

	if limit > maxListLimit {
		limit = maxListLimit
	}

	if offset < 0 {
		offset = 0
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, false, fmt.Errorf("getting property history: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	query := `SELECT id, tenant_id, node_id, property_key, old_value, new_value, changed_at, reason, changed_by
		FROM kg_property_history
		WHERE tenant_id = current_setting('app.tenant_id')::uuid AND node_id = $1`
	args := []any{nodeID}
	argIdx := 2

	if propertyKey != "" {
		query += fmt.Sprintf(" AND property_key = $%d", argIdx)
		args = append(args, propertyKey)
		argIdx++
	}

	query += " ORDER BY changed_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit+1, offset)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("querying property history: %w", err)
	}
	defer rows.Close()

	changes := make([]models.PropertyChange, 0, limit+1)

	for rows.Next() {
		var c models.PropertyChange
		var tenantUUID uuid.UUID

		if err := rows.Scan(
			&c.ID, &tenantUUID, &c.NodeID, &c.PropertyKey,
			&c.OldValue, &c.NewValue, &c.ChangedAt, &c.Reason, &c.ChangedBy,
		); err != nil {
			return nil, false, fmt.Errorf("scanning property history row: %w", err)
		}

		c.TenantID = tenantUUID
		changes = append(changes, c)
	}

	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterating property history rows: %w", err)
	}

	hasMore := len(changes) > limit
	if hasMore {
		changes = changes[:limit]
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing property history query: %w", err)
	}

	return changes, hasMore, nil
}
