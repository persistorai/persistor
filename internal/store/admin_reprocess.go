package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

const defaultReprocessBatchSize = 100

// ReprocessableNode contains the minimum fields needed to rebuild search text and embeddings.
type ReprocessableNode struct {
	ID                 string
	Type               string
	Label              string
	Properties         map[string]any
	NeedsSearchText    bool
	NeedsEmbedding     bool
}

// ListNodesForReprocess returns a batch of nodes ordered by creation time.
func (s *EmbeddingStore) ListNodesForReprocess(ctx context.Context, tenantID string, limit int) ([]ReprocessableNode, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if limit <= 0 {
		limit = defaultReprocessBatchSize
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes for reprocess: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx,
		`SELECT `+nodeColumns+` FROM kg_nodes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND (embedding IS NULL OR search_text = '')
		 ORDER BY created_at
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying nodes for reprocess: %w", err)
	}
	defer rows.Close()

	result := make([]ReprocessableNode, 0, limit)
	for rows.Next() {
		node, err := scanNode(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning nodes for reprocess: %w", err)
		}
		if err := s.decryptNode(ctx, tenantID, node); err != nil {
			return nil, err
		}
		result = append(result, ReprocessableNode{
			ID:              node.ID,
			Type:            node.Type,
			Label:           node.Label,
			Properties:      node.Properties,
			NeedsSearchText: true,
			NeedsEmbedding:  true,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing list nodes for reprocess: %w", err)
	}
	return result, nil
}

// CountNodesForReprocess returns counts for rows still needing search text and/or embeddings.
func (s *EmbeddingStore) CountNodesForReprocess(ctx context.Context, tenantID string) (remainingSearchText, remainingEmbeddings, remainingTotal int, err error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("counting nodes for reprocess: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.QueryRow(ctx,
		`SELECT
			COUNT(*) FILTER (WHERE search_text = '') AS remaining_search_text,
			COUNT(*) FILTER (WHERE embedding IS NULL) AS remaining_embeddings,
			COUNT(*) FILTER (WHERE search_text = '' OR embedding IS NULL) AS remaining_total
		 FROM kg_nodes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid`,
	).Scan(&remainingSearchText, &remainingEmbeddings, &remainingTotal)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("querying reprocess counts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("committing reprocess counts: %w", err)
	}
	return remainingSearchText, remainingEmbeddings, remainingTotal, nil
}

// UpdateNodeSearchText rewrites the search_text document for an existing node.
func (s *EmbeddingStore) UpdateNodeSearchText(ctx context.Context, tenantID, nodeID, searchText string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("updating node search text: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`UPDATE kg_nodes SET search_text = $1 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $2`,
		searchText, nodeID,
	)
	if err != nil {
		return fmt.Errorf("executing node search text update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrNodeNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing node search text update: %w", err)
	}
	return nil
}
