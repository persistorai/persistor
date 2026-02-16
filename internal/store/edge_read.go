package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// buildEdgeListQuery constructs the filtered SELECT query and arguments for ListEdges.
func buildEdgeListQuery(source, target, relation string, limit, offset int) (query string, args []any) {
	where := " WHERE tenant_id = current_setting('app.tenant_id')::uuid"
	filterArgs := make([]any, 0, 3)
	argIdx := 1

	if source != "" {
		where += fmt.Sprintf(" AND source = $%d", argIdx)
		filterArgs = append(filterArgs, source)
		argIdx++
	}

	if target != "" {
		where += fmt.Sprintf(" AND target = $%d", argIdx)
		filterArgs = append(filterArgs, target)
		argIdx++
	}

	if relation != "" {
		where += fmt.Sprintf(" AND relation = $%d", argIdx)
		filterArgs = append(filterArgs, relation)
		argIdx++
	}

	query = "SELECT " + edgeColumns + " FROM kg_edges" + where
	query += " ORDER BY updated_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = make([]any, 0, len(filterArgs)+2)
	args = append(args, filterArgs...)
	args = append(args, limit+1, offset)

	return query, args
}

// ListEdges returns edges for a tenant with optional source, target, and relation filters.
func (s *EdgeStore) ListEdges(
	ctx context.Context,
	tenantID string,
	source, target, relation string,
	limit, offset int,
) ([]models.Edge, bool, error) {
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
		return nil, false, fmt.Errorf("listing edges: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	query, args := buildEdgeListQuery(source, target, relation, limit, offset)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	edges := make([]models.Edge, 0, limit+1)

	for rows.Next() {
		e, err := scanEdge(rows.Scan)
		if err != nil {
			return nil, false, fmt.Errorf("scanning edge row: %w", err)
		}

		edges = append(edges, *e)
	}

	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterating edge rows: %w", err)
	}

	hasMore := len(edges) > limit
	if hasMore {
		edges = edges[:limit]
	}

	if err := s.decryptEdges(ctx, tenantID, edges); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing list edges: %w", err)
	}

	return edges, hasMore, nil
}

// getEdge fetches a single edge within an existing transaction.
func (s *EdgeStore) getEdge(
	ctx context.Context,
	tx pgx.Tx,
	source, target, relation string,
) (*models.Edge, error) {
	query := "SELECT " + edgeColumns +
		" FROM kg_edges WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $1 AND target = $2 AND relation = $3"

	row := tx.QueryRow(ctx, query, source, target, relation)

	e, err := scanEdge(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEdgeNotFound
		}

		return nil, fmt.Errorf("scanning edge: %w", err)
	}

	return e, nil
}
