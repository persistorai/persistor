package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// PatchEdgeProperties merges patch properties into existing edge properties.
func (s *EdgeStore) PatchEdgeProperties(
	ctx context.Context,
	tenantID string,
	source, target, relation string,
	req models.PatchPropertiesRequest,
) (*models.Edge, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("patching edge properties: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Fetch existing properties.
	var propsBytes []byte

	err = tx.QueryRow(ctx,
		`SELECT properties FROM kg_edges WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $1 AND target = $2 AND relation = $3`,
		source, target, relation,
	).Scan(&propsBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEdgeNotFound
		}

		return nil, fmt.Errorf("fetching edge properties: %w", err)
	}

	oldProps, err := s.decryptPropertiesRaw(ctx, tenantID, propsBytes)
	if err != nil {
		return nil, fmt.Errorf("decrypting edge properties: %w", err)
	}

	merged := models.MergeProperties(oldProps, req.Properties)

	encProps, err := s.encryptProperties(ctx, tenantID, merged)
	if err != nil {
		return nil, fmt.Errorf("preparing patched edge properties: %w", err)
	}

	query := fmt.Sprintf(
		"UPDATE kg_edges SET properties = $1 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $2 AND target = $3 AND relation = $4 RETURNING %s",
		edgeColumns,
	)

	row := tx.QueryRow(ctx, query, encProps, source, target, relation)

	e, err := scanEdge(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEdgeNotFound
		}

		return nil, fmt.Errorf("scanning patched edge: %w", err)
	}

	if err := s.decryptEdge(ctx, tenantID, e); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing patch edge properties: %w", err)
	}

	s.notify("kg_edges", "update", tenantID)

	return e, nil
}

// DeleteEdge removes an edge by its composite key.
func (s *EdgeStore) DeleteEdge(
	ctx context.Context,
	tenantID string,
	source, target, relation string,
) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("deleting edge: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	tag, err := tx.Exec(ctx,
		"DELETE FROM kg_edges WHERE tenant_id = $1 AND source = $2 AND target = $3 AND relation = $4",
		tenantID, source, target, relation,
	)
	if err != nil {
		return fmt.Errorf("executing edge delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrEdgeNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing delete edge: %w", err)
	}

	s.notify("kg_edges", "delete", tenantID)

	return nil
}
