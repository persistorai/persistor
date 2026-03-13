package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// RelationTypeRow represents a relation type row from the database.
type RelationTypeRow struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// RelationTypeStore provides relation type CRUD operations.
type RelationTypeStore struct {
	Base
}

// NewRelationTypeStore creates a new RelationTypeStore.
func NewRelationTypeStore(base Base) *RelationTypeStore {
	return &RelationTypeStore{Base: base}
}

// ListRelationTypes returns all global and tenant-specific relation types.
func (s *RelationTypeStore) ListRelationTypes(
	ctx context.Context,
	tenantID string,
) ([]RelationTypeRow, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing relation types: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	rows, err := tx.Query(ctx,
		`SELECT name, description, tenant_id, created_at
		 FROM relation_types
		 WHERE tenant_id IS NULL OR tenant_id = $1
		 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying relation types: %w", err)
	}

	result, err := scanRelationTypeRows(rows)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing list relation types: %w", err)
	}

	return result, nil
}

// AddRelationType inserts a tenant-specific relation type.
func (s *RelationTypeStore) AddRelationType(
	ctx context.Context,
	tenantID string,
	name string,
	description string,
) (*RelationTypeRow, error) {
	if name == "" {
		return nil, fmt.Errorf("relation type name: %w", models.ErrMissingLabel)
	}

	if description == "" {
		return nil, fmt.Errorf("relation type description: %w", models.ErrMissingLabel)
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("adding relation type: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	row := tx.QueryRow(ctx,
		`INSERT INTO relation_types (name, description, tenant_id)
		 VALUES ($1, $2, $3)
		 RETURNING name, description, tenant_id, created_at`,
		name, description, tenantID)

	rt, err := scanRelationTypeRow(row)
	if err != nil {
		return nil, fmt.Errorf("inserting relation type: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing add relation type: %w", err)
	}

	return rt, nil
}

// IsCanonical checks whether a relation type name exists as a global (canonical) type.
func (s *RelationTypeStore) IsCanonical(
	ctx context.Context,
	name string,
) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var exists bool

	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM relation_types WHERE name = $1 AND tenant_id IS NULL)`,
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking canonical relation type: %w", err)
	}

	return exists, nil
}

// scanRelationTypeRow scans a single relation type row.
func scanRelationTypeRow(row pgx.Row) (*RelationTypeRow, error) {
	var rt RelationTypeRow

	err := row.Scan(&rt.Name, &rt.Description, &rt.TenantID, &rt.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning relation type row: %w", err)
	}

	return &rt, nil
}

// scanRelationTypeRows collects all rows from a relation type query.
func scanRelationTypeRows(rows pgx.Rows) ([]RelationTypeRow, error) {
	defer rows.Close()

	var result []RelationTypeRow

	for rows.Next() {
		var rt RelationTypeRow

		err := rows.Scan(&rt.Name, &rt.Description, &rt.TenantID, &rt.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning relation type rows: %w", err)
		}

		result = append(result, rt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating relation type rows: %w", err)
	}

	return result, nil
}
