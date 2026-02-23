package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/persistorai/persistor/internal/models"
)

// parseEmbedding converts a pgvector string "[0.1,0.2,...]" back to []float32.
func parseEmbedding(s string) []float32 {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))

	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			continue
		}

		out = append(out, float32(v))
	}

	return out
}

// ExportStore handles export and import operations for the knowledge graph.
type ExportStore struct {
	Base
}

// NewExportStore creates a new ExportStore.
func NewExportStore(base Base) *ExportStore {
	return &ExportStore{Base: base}
}

// ExportAllNodes reads all nodes for a tenant with full fidelity.
// Properties are decrypted before returning for portable export.
// Embeddings and access metrics are included for backup/restore.
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
		       embedding, access_count, last_accessed,
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
		var embeddingStr *string

		if err := rows.Scan(
			&n.ID, &n.Type, &n.Label, &propsBytes,
			&embeddingStr, &n.AccessCount, &n.LastAccessed,
			&n.SalienceScore, &n.UserBoosted, &n.SupersededBy,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning export node: %w", err)
		}

		if embeddingStr != nil {
			n.Embedding = parseEmbedding(*embeddingStr)
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
		       weight, access_count, last_accessed,
		       created_at, updated_at
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
			&e.Weight, &e.AccessCount, &e.LastAccessed,
			&e.CreatedAt, &e.UpdatedAt,
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
