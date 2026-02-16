package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// salienceFormula is the SQL expression for computing salience_score.
const salienceFormula = `GREATEST(0.1,
	1.0
	+ log(2.0, access_count + 1) * 0.3
	+ GREATEST(0, 1 - EXTRACT(EPOCH FROM (NOW() - COALESCE(last_accessed, created_at))) / (180 * 86400)) * 0.5
	+ CASE WHEN user_boosted THEN 2.0 ELSE 0 END
	- CASE WHEN superseded_by IS NOT NULL THEN 0.5 ELSE 0 END
)`

// salienceBatchSize is the number of rows to update per batch during recalculation.
const salienceBatchSize = 1000

// SalienceStore handles salience scoring operations.
type SalienceStore struct {
	Base
}

// NewSalienceStore creates a new SalienceStore.
func NewSalienceStore(base Base) *SalienceStore {
	return &SalienceStore{Base: base}
}

// BoostNode sets user_boosted to TRUE and recalculates the salience score.
// Returns the updated node, or nil if not found.
func (s *SalienceStore) BoostNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("boosting node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	sql := `UPDATE kg_nodes
		SET user_boosted = TRUE,
			salience_score = ` + salienceFormula + `
		WHERE tenant_id = $1 AND id = $2
		RETURNING ` + nodeColumns

	row := tx.QueryRow(ctx, sql, tenantID, nodeID)

	n, err := scanNode(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNodeNotFound
		}

		return nil, fmt.Errorf("scanning boosted node: %w", err)
	}

	if err := s.decryptNode(ctx, tenantID, n); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing boost node: %w", err)
	}

	return n, nil
}

// SupersedeNode marks oldID as superseded by newID and recalculates salience
// for both nodes in a single transaction.
func (s *SalienceStore) SupersedeNode(
	ctx context.Context,
	tenantID, oldID, newID string,
) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("superseding node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	oldSQL := `UPDATE kg_nodes
		SET superseded_by = $3,
			salience_score = ` + salienceFormula + `
		WHERE tenant_id = $1 AND id = $2`

	tag, err := tx.Exec(ctx, oldSQL, tenantID, oldID, newID)
	if err != nil {
		return fmt.Errorf("marking node superseded: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrNodeNotFound
	}

	newSQL := `UPDATE kg_nodes
		SET salience_score = ` + salienceFormula + `
		WHERE tenant_id = $1 AND id = $2`

	newTag, err := tx.Exec(ctx, newSQL, tenantID, newID)
	if err != nil {
		return fmt.Errorf("recalculating new node salience: %w", err)
	}

	if newTag.RowsAffected() == 0 {
		return fmt.Errorf("new node %s: %w", newID, models.ErrNodeNotFound)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing supersede: %w", err)
	}

	return nil
}

// RecalculateSalience recomputes salience_score for all nodes belonging to
// the given tenant in cursor-based batches. Returns the number of updated nodes.
func (s *SalienceStore) RecalculateSalience(ctx context.Context, tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	total := 0
	lastID := ""

	for {
		n, newLastID, err := s.recalculateSalienceBatchCursor(ctx, tenantID, lastID)
		if err != nil {
			return total, err
		}

		total += n

		if newLastID == "" || newLastID == lastID {
			break
		}
		lastID = newLastID
	}

	saliencePayload, _ := json.Marshal(map[string]any{ //nolint:errcheck // static keys, cannot fail.
		"event":     "salience_recalculated",
		"tenant_id": tenantID,
	})
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer notifyCancel()
	if _, err := s.Pool.Exec(notifyCtx, "SELECT pg_notify('kg_changes', $1)", string(saliencePayload)); err != nil {
		s.Log.WithError(err).Warn("failed to send salience recalculation notification")
	}

	return total, nil
}

// recalculateSalienceBatchCursor processes nodes with id > lastID using
// cursor-based pagination. Returns updated count and the last processed ID.
func (s *SalienceStore) recalculateSalienceBatchCursor(ctx context.Context, tenantID, lastID string) (updated int, newCursor string, err error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return 0, lastID, fmt.Errorf("recalculating salience: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	batchSQL := `WITH batch AS (
			SELECT id, salience_score AS old_score,
				(` + salienceFormula + `) AS new_score
			FROM kg_nodes
			WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id > $1
			ORDER BY id
			LIMIT $2
		),
		updated AS (
			UPDATE kg_nodes n
			SET salience_score = b.new_score
			FROM batch b
			WHERE n.id = b.id AND n.tenant_id = current_setting('app.tenant_id')::uuid AND b.old_score IS DISTINCT FROM b.new_score
			RETURNING n.id
		)
		SELECT COALESCE((SELECT max(id) FROM batch), ''), (SELECT count(*) FROM updated)`

	var newLastID string
	var updatedCount int64
	err = tx.QueryRow(ctx, batchSQL, lastID, salienceBatchSize).Scan(&newLastID, &updatedCount)
	if err != nil {
		return 0, lastID, fmt.Errorf("executing salience recalculation batch: %w", err)
	}

	if newLastID == "" {
		if err := tx.Commit(ctx); err != nil {
			return 0, lastID, fmt.Errorf("committing salience batch: %w", err)
		}
		return 0, lastID, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, lastID, fmt.Errorf("committing salience recalculation batch: %w", err)
	}

	return int(updatedCount), newLastID, nil
}
