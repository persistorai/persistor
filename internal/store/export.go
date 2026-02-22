package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// ExportStore handles export and import operations for the knowledge graph.
type ExportStore struct {
	Base
}

// NewExportStore creates a new ExportStore.
func NewExportStore(base Base) *ExportStore {
	return &ExportStore{Base: base}
}

// ExportAllNodes reads all nodes for a tenant, excluding embeddings.
// Properties are decrypted before returning for portable export.
// Returns nodes sorted by created_at, id for deterministic exports.
func (s *ExportStore) ExportAllNodes(ctx context.Context, tenantID string) ([]models.ExportNode, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("export nodes: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	rows, err := tx.Query(ctx, `
		SELECT id, type, label, properties,
		       salience_score, user_boosted, superseded_by,
		       created_at, updated_at
		FROM kg_nodes
		WHERE tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("querying nodes for export: %w", err)
	}

	defer rows.Close()

	var nodes []models.ExportNode

	for rows.Next() {
		var n models.ExportNode
		var propsBytes []byte

		if err := rows.Scan(
			&n.ID, &n.Type, &n.Label, &propsBytes,
			&n.SalienceScore, &n.UserBoosted, &n.SupersededBy,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning export node: %w", err)
		}

		props, err := s.decryptPropertiesRaw(ctx, tenantID, propsBytes)
		if err != nil {
			return nil, fmt.Errorf("decrypting node %s properties: %w", n.ID, err)
		}

		n.Properties = props
		nodes = append(nodes, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating export nodes: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing export nodes: %w", err)
	}

	return nodes, nil
}

// ExportAllEdges reads all edges for a tenant.
// Properties are decrypted before returning for portable export.
// Returns edges sorted by (source, target, relation) for deterministic exports.
func (s *ExportStore) ExportAllEdges(ctx context.Context, tenantID string) ([]models.ExportEdge, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("export edges: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	rows, err := tx.Query(ctx, `
		SELECT source, target, relation, properties,
		       weight, created_at, updated_at
		FROM kg_edges
		WHERE tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY source, target, relation
	`)
	if err != nil {
		return nil, fmt.Errorf("querying edges for export: %w", err)
	}

	defer rows.Close()

	var edges []models.ExportEdge

	for rows.Next() {
		var e models.ExportEdge
		var propsBytes []byte

		if err := rows.Scan(
			&e.Source, &e.Target, &e.Relation, &propsBytes,
			&e.Weight, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning export edge: %w", err)
		}

		props, err := s.decryptPropertiesRaw(ctx, tenantID, propsBytes)
		if err != nil {
			return nil, fmt.Errorf("decrypting edge %s→%s properties: %w", e.Source, e.Target, err)
		}

		e.Properties = props
		edges = append(edges, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating export edges: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing export edges: %w", err)
	}

	return edges, nil
}
