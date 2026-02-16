package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// SearchStore handles full-text, semantic, and hybrid search queries.
type SearchStore struct {
	Base
}

// NewSearchStore creates a new SearchStore.
func NewSearchStore(base Base) *SearchStore {
	return &SearchStore{Base: base}
}

// FullTextSearch searches nodes using PostgreSQL full-text search with optional
// type and salience filters. Results are ranked by text relevance and salience.
func (s *SearchStore) FullTextSearch(
	ctx context.Context,
	tenantID string,
	query string,
	typeFilter string,
	minSalience float64,
	limit int,
) ([]models.Node, error) {
	if limit <= 0 {
		limit = 20
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("full-text search: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	sql := `SELECT ` + nodeColumns + `
		FROM kg_nodes
		WHERE label_tsv @@ plainto_tsquery('english', $1)
			AND tenant_id = current_setting('app.tenant_id')::uuid`

	args := []any{query}
	argIdx := 2

	if typeFilter != "" {
		sql += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, typeFilter)
		argIdx++
	}

	if minSalience > 0 {
		sql += fmt.Sprintf(" AND salience_score >= $%d", argIdx)
		args = append(args, minSalience)
		argIdx++
	}

	sql += fmt.Sprintf(` ORDER BY ts_rank(label_tsv,
			plainto_tsquery('english', $1)) DESC, salience_score DESC LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("executing full-text search: %w", err)
	}
	defer rows.Close()

	nodes, err := collectNodes(rows)
	if err != nil {
		return nil, err
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing full-text search: %w", err)
	}

	return nodes, nil
}

// SemanticSearch finds nodes similar to the given embedding vector using
// pgvector cosine distance. The embedding must be pre-computed.
func (s *SearchStore) SemanticSearch(
	ctx context.Context,
	tenantID string,
	embedding []float32,
	limit int,
) ([]models.ScoredNode, error) {
	if limit <= 0 {
		limit = 10
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	embeddingStr := formatEmbedding(embedding)

	sql := `SELECT ` + nodeColumns + `, 1 - (embedding <=> $1::vector) AS similarity
		FROM kg_nodes
		WHERE embedding IS NOT NULL
			AND tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY embedding <=> $1::vector
		LIMIT $2`

	rows, err := tx.Query(ctx, sql, embeddingStr, limit)
	if err != nil {
		return nil, fmt.Errorf("executing semantic search: %w", err)
	}
	defer rows.Close()

	scored := make([]models.ScoredNode, 0, limit)
	for rows.Next() {
		var score float64
		n, err := scanNode(func(dest ...any) error {
			return rows.Scan(append(dest, &score)...) //nolint:gocritic // append to extend scan targets
		})
		if err != nil {
			return nil, fmt.Errorf("scanning semantic result: %w", err)
		}
		scored = append(scored, models.ScoredNode{Node: *n, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating semantic rows: %w", err)
	}

	// Decrypt node properties.
	nodes := make([]models.Node, len(scored))
	for i := range scored {
		nodes[i] = scored[i].Node
	}
	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}
	for i := range scored {
		scored[i].Node = nodes[i]
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing semantic search: %w", err)
	}

	return scored, nil
}

// HybridSearch combines full-text and vector similarity search using
// Reciprocal Rank Fusion (RRF) to merge the ranked result lists.
func (s *SearchStore) HybridSearch(
	ctx context.Context,
	tenantID string,
	query string,
	embedding []float32,
	limit int,
) ([]models.Node, error) {
	if limit <= 0 {
		limit = 10
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	embeddingStr := formatEmbedding(embedding)

	sql := `WITH fts AS (
			SELECT id, tenant_id, ts_rank(label_tsv, plainto_tsquery('english', $1)) AS rank
			FROM kg_nodes
			WHERE label_tsv @@ plainto_tsquery('english', $1)
				AND tenant_id = current_setting('app.tenant_id')::uuid
			ORDER BY rank DESC
			LIMIT $3
		),
		vec AS (
			SELECT id, tenant_id, embedding <=> $2::vector AS dist
			FROM kg_nodes
			WHERE embedding IS NOT NULL
				AND tenant_id = current_setting('app.tenant_id')::uuid
			ORDER BY dist
			LIMIT $3
		),
		ranked_fts AS (
			SELECT id, tenant_id, 1.0 / (60 + ROW_NUMBER() OVER (ORDER BY rank DESC)) AS rrf FROM fts
		),
		ranked_vec AS (
			SELECT id, tenant_id, 1.0 / (60 + ROW_NUMBER() OVER (ORDER BY dist)) AS rrf FROM vec
		),
		combined AS (
			SELECT COALESCE(f.id, v.id) AS id,
				COALESCE(f.tenant_id, v.tenant_id) AS tenant_id,
				COALESCE(f.rrf, 0) + COALESCE(v.rrf, 0) AS rrf_score
			FROM ranked_fts f
			FULL OUTER JOIN ranked_vec v ON f.tenant_id = v.tenant_id AND f.id = v.id
		)
		SELECT n.id, n.tenant_id, n.type, n.label, n.properties,
			n.access_count, n.last_accessed, n.salience_score, n.superseded_by,
			n.user_boosted, n.created_at, n.updated_at
		FROM kg_nodes n
		INNER JOIN combined c ON n.tenant_id = c.tenant_id AND n.id = c.id
		WHERE n.tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY c.rrf_score DESC
		LIMIT $3`

	rows, err := tx.Query(ctx, sql, query, embeddingStr, limit)
	if err != nil {
		return nil, fmt.Errorf("executing hybrid search: %w", err)
	}
	defer rows.Close()

	nodes, err := collectNodes(rows)
	if err != nil {
		return nil, err
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing hybrid search: %w", err)
	}

	return nodes, nil
}
