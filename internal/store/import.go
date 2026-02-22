package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// UpsertNodeFromExport creates or updates a node from an export record.
// If the node exists and overwrite is true, updates type/label/properties and
// salience fields. If the node exists and overwrite is false, skips.
// Returns "created", "updated", or "skipped".
func (s *ExportStore) UpsertNodeFromExport(
	ctx context.Context,
	tenantID string,
	node models.ExportNode,
	overwrite bool,
) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	props := node.Properties
	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := s.encryptProperties(ctx, tenantID, props)
	if err != nil {
		return "", fmt.Errorf("encrypting node properties: %w", err)
	}

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("upsert node from export: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	var action string

	if overwrite {
		action, err = upsertNodeOverwrite(ctx, tx, tenantID, node, propsJSON)
	} else {
		action, err = upsertNodeSkip(ctx, tx, tenantID, node, propsJSON)
	}

	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("committing node upsert: %w", err)
	}

	return action, nil
}

func upsertNodeOverwrite(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	node models.ExportNode,
	propsJSON []byte,
) (string, error) {
	var wasInserted bool

	err := tx.QueryRow(ctx, `
		INSERT INTO kg_nodes
			(id, tenant_id, type, label, properties,
			 salience_score, user_boosted, superseded_by,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, id) DO UPDATE SET
			type          = EXCLUDED.type,
			label         = EXCLUDED.label,
			properties    = EXCLUDED.properties,
			salience_score = EXCLUDED.salience_score,
			user_boosted  = EXCLUDED.user_boosted,
			superseded_by = EXCLUDED.superseded_by,
			updated_at    = EXCLUDED.updated_at
		RETURNING (xmax = 0) AS was_inserted
	`,
		node.ID, tenantID, node.Type, node.Label, propsJSON,
		node.SalienceScore, node.UserBoosted, node.SupersededBy,
		node.CreatedAt, node.UpdatedAt,
	).Scan(&wasInserted)
	if err != nil {
		return "", fmt.Errorf("upserting node: %w", err)
	}

	if wasInserted {
		return "created", nil
	}

	return "updated", nil
}

func upsertNodeSkip(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	node models.ExportNode,
	propsJSON []byte,
) (string, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO kg_nodes
			(id, tenant_id, type, label, properties,
			 salience_score, user_boosted, superseded_by,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, id) DO NOTHING
	`,
		node.ID, tenantID, node.Type, node.Label, propsJSON,
		node.SalienceScore, node.UserBoosted, node.SupersededBy,
		node.CreatedAt, node.UpdatedAt,
	)
	if err != nil {
		return "", fmt.Errorf("inserting node: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return "skipped", nil
	}

	return "created", nil
}

// UpsertEdgeFromExport creates or updates an edge from an export record.
// Same overwrite/skip semantics as UpsertNodeFromExport.
// Returns "created", "updated", or "skipped".
func (s *ExportStore) UpsertEdgeFromExport(
	ctx context.Context,
	tenantID string,
	edge models.ExportEdge,
	overwrite bool,
) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	props := edge.Properties
	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := s.encryptProperties(ctx, tenantID, props)
	if err != nil {
		return "", fmt.Errorf("encrypting edge properties: %w", err)
	}

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("upsert edge from export: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	var action string

	if overwrite {
		action, err = upsertEdgeOverwrite(ctx, tx, tenantID, edge, propsJSON)
	} else {
		action, err = upsertEdgeSkip(ctx, tx, tenantID, edge, propsJSON)
	}

	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("committing edge upsert: %w", err)
	}

	return action, nil
}

func upsertEdgeOverwrite(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	edge models.ExportEdge,
	propsJSON []byte,
) (string, error) {
	var wasInserted bool

	err := tx.QueryRow(ctx, `
		INSERT INTO kg_edges
			(tenant_id, source, target, relation, properties,
			 weight, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, source, target, relation) DO UPDATE SET
			properties = EXCLUDED.properties,
			weight     = EXCLUDED.weight,
			updated_at = EXCLUDED.updated_at
		RETURNING (xmax = 0) AS was_inserted
	`,
		tenantID, edge.Source, edge.Target, edge.Relation, propsJSON,
		edge.Weight, edge.CreatedAt, edge.UpdatedAt,
	).Scan(&wasInserted)
	if err != nil {
		return "", fmt.Errorf("upserting edge: %w", err)
	}

	if wasInserted {
		return "created", nil
	}

	return "updated", nil
}

func upsertEdgeSkip(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	edge models.ExportEdge,
	propsJSON []byte,
) (string, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO kg_edges
			(tenant_id, source, target, relation, properties,
			 weight, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, source, target, relation) DO NOTHING
	`,
		tenantID, edge.Source, edge.Target, edge.Relation, propsJSON,
		edge.Weight, edge.CreatedAt, edge.UpdatedAt,
	)
	if err != nil {
		return "", fmt.Errorf("inserting edge: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return "skipped", nil
	}

	return "created", nil
}
