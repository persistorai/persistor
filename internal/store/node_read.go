package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// ListNodes returns nodes for a tenant with optional type filter and minimum salience.
func (s *NodeStore) ListNodes(
	ctx context.Context,
	tenantID string,
	typeFilter string,
	minSalience float64,
	limit, offset int,
) ([]models.Node, bool, error) {
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
		return nil, false, fmt.Errorf("listing nodes: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	where := " WHERE tenant_id = current_setting('app.tenant_id')::uuid"
	filterArgs := make([]any, 0, 2)
	argIdx := 1

	if typeFilter != "" {
		where += fmt.Sprintf(" AND type = $%d", argIdx)
		filterArgs = append(filterArgs, typeFilter)
		argIdx++
	}

	if minSalience > 0 {
		where += fmt.Sprintf(" AND salience_score >= $%d", argIdx)
		filterArgs = append(filterArgs, minSalience)
		argIdx++
	}

	query := "SELECT " + nodeColumns + " FROM kg_nodes" + where
	query += " ORDER BY salience_score DESC, updated_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args := make([]any, 0, len(filterArgs)+2)
	args = append(args, filterArgs...)
	args = append(args, limit+1, offset)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("querying nodes: %w", err)
	}
	defer rows.Close()

	nodes := make([]models.Node, 0, limit+1)

	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, false, fmt.Errorf("scanning node row: %w", err)
		}

		nodes = append(nodes, *n)
	}

	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterating node rows: %w", err)
	}

	hasMore := len(nodes) > limit
	if hasMore {
		nodes = nodes[:limit]
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing list nodes: %w", err)
	}

	return nodes, hasMore, nil
}

// GetNode retrieves a single node by ID (pure read, no side effects).
func (s *NodeStore) GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	query := `SELECT ` + nodeColumns + ` FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`

	row := tx.QueryRow(ctx, query, nodeID)

	n, err := scanNode(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNodeNotFound
		}

		return nil, fmt.Errorf("scanning node: %w", err)
	}

	if err := s.decryptNode(ctx, tenantID, n); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing get node: %w", err)
	}

	return n, nil
}
