package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/persistorai/persistor/internal/models"
)

const defaultReprocessBatchSize = 100

// ReprocessableNode contains the minimum fields needed to rebuild search text and embeddings.
type ReprocessableNode struct {
	ID                 string
	Type               string
	Label              string
	Properties         map[string]any
	CurrentSearchText  string
	NeedsSearchText    bool
	NeedsEmbedding     bool
	HasFactEvidence    bool
	HasSupersededFacts bool
	NodeSuperseded     bool
}

// ListNodesForReprocess returns a batch of nodes ordered by creation time.
func (s *EmbeddingStore) ListNodesForReprocess(ctx context.Context, tenantID string, limit int) ([]ReprocessableNode, error) {
	return s.listNodesForMaintenance(ctx, tenantID, limit, false)
}

// ListNodesForMaintenance returns nodes that need explicit maintenance work.
func (s *EmbeddingStore) ListNodesForMaintenance(ctx context.Context, tenantID string, limit int) ([]ReprocessableNode, error) {
	return s.listNodesForMaintenance(ctx, tenantID, limit, true)
}

func (s *EmbeddingStore) listNodesForMaintenance(ctx context.Context, tenantID string, limit int, includeFactEvidence bool) ([]ReprocessableNode, error) {
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
		return nil, fmt.Errorf("listing nodes for maintenance: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	query := `SELECT ` + nodeColumns + `, search_text, embedding IS NULL,
		(properties ? $2) AS has_fact_evidence,
		(superseded_by IS NOT NULL) AS node_superseded
		FROM kg_nodes
		WHERE tenant_id = current_setting('app.tenant_id')::uuid
		  AND (
			embedding IS NULL
			OR search_text = ''
			OR ($1 AND properties ? $2)
			OR superseded_by IS NOT NULL
		  )
		ORDER BY updated_at DESC, created_at DESC
		LIMIT $3`
	rows, err := tx.Query(ctx, query, includeFactEvidence, models.FactEvidenceProperty, limit)
	if err != nil {
		return nil, fmt.Errorf("querying nodes for maintenance: %w", err)
	}
	defer rows.Close()

	result := make([]ReprocessableNode, 0, limit)
	for rows.Next() {
		var (
			node            models.Node
			tenantUUID      uuid.UUID
			props           []byte
			currentSearch   string
			needsEmbedding  bool
			hasFactEvidence bool
			nodeSuperseded  bool
		)
		if err := rows.Scan(
			&node.ID,
			&tenantUUID,
			&node.Type,
			&node.Label,
			&props,
			&node.AccessCount,
			&node.LastAccessed,
			&node.Salience,
			&node.SupersededBy,
			&node.UserBoosted,
			&node.CreatedAt,
			&node.UpdatedAt,
			&currentSearch,
			&needsEmbedding,
			&hasFactEvidence,
			&nodeSuperseded,
		); err != nil {
			return nil, fmt.Errorf("scanning nodes for maintenance: %w", err)
		}
		node.TenantID = tenantUUID
		if err := json.Unmarshal(props, &node.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling node properties: %w", err)
		}
		if err := s.decryptNode(ctx, tenantID, &node); err != nil {
			return nil, err
		}
		result = append(result, ReprocessableNode{
			ID:                 node.ID,
			Type:               node.Type,
			Label:              node.Label,
			Properties:         node.Properties,
			CurrentSearchText:  currentSearch,
			NeedsSearchText:    currentSearch == "",
			NeedsEmbedding:     needsEmbedding,
			HasFactEvidence:    hasFactEvidence,
			HasSupersededFacts: hasSupersededFactEvidence(node.Properties),
			NodeSuperseded:     nodeSuperseded,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing list nodes for maintenance: %w", err)
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

func hasSupersededFactEvidence(properties map[string]any) bool {
	raw, ok := properties[models.FactEvidenceProperty]
	if !ok || raw == nil {
		return false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var evidence map[string][]models.FactEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return false
	}
	for _, entries := range evidence {
		for _, entry := range entries {
			if entry.SupersedesPrior || entry.ConflictsWithPrior || entry.HistoricalEvidenceRetained || entry.PreviousValue != nil {
				return true
			}
		}
	}
	return false
}
