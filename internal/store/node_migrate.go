package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/persistorai/persistor/internal/models"
)

// MigrateNode atomically migrates a node to a new ID, updating all edges.
func (s *NodeStore) MigrateNode(
	ctx context.Context,
	tenantID string,
	oldID string,
	req models.MigrateNodeRequest,
) (*models.MigrateNodeResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("migrate node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// 1. Read old node.
	row := tx.QueryRow(ctx,
		`SELECT `+nodeColumns+` FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
		oldID)

	oldNode, err := scanNode(row.Scan)
	if err != nil {
		return nil, models.ErrNodeNotFound
	}

	// 2. Determine label for new node.
	label := oldNode.Label
	if req.NewLabel != "" {
		label = req.NewLabel
	}

	// 3. Read encrypted properties from old node (raw bytes for copying).
	var rawProps []byte
	err = tx.QueryRow(ctx,
		`SELECT properties FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
		oldID).Scan(&rawProps)
	if err != nil {
		return nil, fmt.Errorf("reading old node properties: %w", err)
	}

	// 4. Create new node copying all fields.
	_, err = tx.Exec(ctx,
		`INSERT INTO kg_nodes (id, tenant_id, type, label, properties, salience_score, access_count, last_accessed, user_boosted)
		 SELECT $1, tenant_id, type, $2, properties, salience_score, access_count, last_accessed, user_boosted
		 FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $3`,
		req.NewID, label, oldID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}

		return nil, fmt.Errorf("creating new node: %w", err)
	}

	// 5. Copy embedding if present.
	_, err = tx.Exec(ctx,
		`UPDATE kg_nodes SET embedding = old.embedding
		 FROM kg_nodes old
		 WHERE kg_nodes.id = $1
		   AND kg_nodes.tenant_id = current_setting('app.tenant_id')::uuid
		   AND old.id = $2
		   AND old.tenant_id = current_setting('app.tenant_id')::uuid
		   AND old.embedding IS NOT NULL`,
		req.NewID, oldID)
	if err != nil {
		return nil, fmt.Errorf("copying embedding: %w", err)
	}

	// 6. Update outgoing edges.
	tagOut, err := tx.Exec(ctx,
		`UPDATE kg_edges SET source = $1 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $2`,
		req.NewID, oldID)
	if err != nil {
		return nil, fmt.Errorf("migrating outgoing edges: %w", err)
	}

	// 7. Update incoming edges.
	tagIn, err := tx.Exec(ctx,
		`UPDATE kg_edges SET target = $1 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND target = $2`,
		req.NewID, oldID)
	if err != nil {
		return nil, fmt.Errorf("migrating incoming edges: %w", err)
	}

	result := &models.MigrateNodeResult{
		OldID:         oldID,
		NewID:         req.NewID,
		OutgoingEdges: int(tagOut.RowsAffected()),
		IncomingEdges: int(tagIn.RowsAffected()),
		Salience:      oldNode.Salience,
		OldDeleted:    false,
	}

	// 8. Delete old node if requested.
	if req.DeleteOld {
		_, err = tx.Exec(ctx,
			`DELETE FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
			oldID)
		if err != nil {
			return nil, fmt.Errorf("deleting old node: %w", err)
		}

		result.OldDeleted = true
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing migrate node: %w", err)
	}

	s.notify("kg_nodes", "update", tenantID)

	return result, nil
}
