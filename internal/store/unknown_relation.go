package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// UnknownRelationStore provides CRUD operations for unknown relations.
type UnknownRelationStore struct {
	Base
}

// NewUnknownRelationStore creates a new UnknownRelationStore.
func NewUnknownRelationStore(base Base) *UnknownRelationStore {
	return &UnknownRelationStore{Base: base}
}

// LogUnknownRelation upserts an unknown relation, incrementing count if it already exists.
func (s *UnknownRelationStore) LogUnknownRelation(
	ctx context.Context,
	tenantID, relationType, sourceName, targetName, sourceText string,
) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("logging unknown relation: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	query := `INSERT INTO unknown_relations (tenant_id, relation_type, source_name, target_name, source_text)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, relation_type, source_name, target_name)
		DO UPDATE SET count = unknown_relations.count + 1, last_seen = now()`

	_, err = tx.Exec(ctx, query, tenantID, relationType, sourceName, targetName, sourceText)
	if err != nil {
		return fmt.Errorf("upserting unknown relation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing unknown relation log: %w", err)
	}

	return nil
}

// ListUnknownRelations returns unknown relations sorted by count descending.
func (s *UnknownRelationStore) ListUnknownRelations(
	ctx context.Context,
	tenantID string,
	opts models.UnknownRelationListOpts,
) ([]models.UnknownRelation, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing unknown relations: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	query := `SELECT id, tenant_id, relation_type, source_name, target_name,
			source_text, count, first_seen, last_seen, resolved, resolved_as
		FROM unknown_relations
		WHERE resolved = $1
		ORDER BY count DESC
		LIMIT $2 OFFSET $3`

	rows, err := tx.Query(ctx, query, opts.ResolvedOnly, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("querying unknown relations: %w", err)
	}
	defer rows.Close()

	results, err := scanUnknownRelations(rows)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing list unknown relations: %w", err)
	}

	return results, nil
}

// ResolveUnknownRelation marks an unknown relation as resolved with a canonical type.
func (s *UnknownRelationStore) ResolveUnknownRelation(
	ctx context.Context,
	tenantID, id, canonicalType string,
) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("resolving unknown relation: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	tag, err := tx.Exec(ctx,
		`UPDATE unknown_relations SET resolved = TRUE, resolved_as = $1
		WHERE tenant_id = $2 AND id = $3`,
		canonicalType, tenantID, id,
	)
	if err != nil {
		return fmt.Errorf("executing resolve unknown relation: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrUnknownRelationNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing resolve unknown relation: %w", err)
	}

	return nil
}

// scanUnknownRelations scans rows into a slice of UnknownRelation.
func scanUnknownRelations(rows pgx.Rows) ([]models.UnknownRelation, error) {
	var results []models.UnknownRelation

	for rows.Next() {
		var ur models.UnknownRelation
		var sourceText *string

		err := rows.Scan(
			&ur.ID, &ur.TenantID, &ur.RelationType,
			&ur.SourceName, &ur.TargetName, &sourceText,
			&ur.Count, &ur.FirstSeen, &ur.LastSeen,
			&ur.Resolved, &ur.ResolvedAs,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning unknown relation row: %w", err)
		}

		if sourceText != nil {
			ur.SourceText = *sourceText
		}

		results = append(results, ur)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unknown relation rows: %w", err)
	}

	return results, nil
}
