package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// EmbeddingStore handles vector embedding operations.
type EmbeddingStore struct {
	Base
}

// NewEmbeddingStore creates a new EmbeddingStore.
func NewEmbeddingStore(base Base) *EmbeddingStore {
	return &EmbeddingStore{Base: base}
}

// UpdateNodeEmbedding sets the embedding vector for a single node.
func (s *EmbeddingStore) UpdateNodeEmbedding(ctx context.Context, tenantID, nodeID string, embedding []float32) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("updating node embedding: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	tag, err := tx.Exec(ctx,
		`UPDATE kg_nodes SET embedding = $1::vector
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $2`,
		formatEmbedding(embedding), nodeID,
	)
	if err != nil {
		return fmt.Errorf("executing embedding update: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrNodeNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing embedding update: %w", err)
	}

	return nil
}

// ListNodesWithoutEmbeddings returns node IDs, types, and labels for nodes
// that have a NULL embedding vector, up to the given limit.
func (s *EmbeddingStore) ListNodesWithoutEmbeddings(ctx context.Context, tenantID string, limit int) ([]models.NodeSummary, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}

	if limit > maxListLimit {
		limit = maxListLimit
	}

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes without embeddings: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // read-only tx, rollback is cleanup.

	rows, err := tx.Query(ctx,
		`SELECT id, type, label FROM kg_nodes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND embedding IS NULL
		 ORDER BY created_at
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying nodes without embeddings: %w", err)
	}

	defer rows.Close()

	var summaries []models.NodeSummary

	for rows.Next() {
		var s models.NodeSummary
		if err := rows.Scan(&s.ID, &s.Type, &s.Label); err != nil {
			return nil, fmt.Errorf("scanning node summary: %w", err)
		}

		summaries = append(summaries, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating node summaries: %w", err)
	}

	return summaries, nil
}
