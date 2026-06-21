package index

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// lockNote reads a note's current state FOR UPDATE so a concurrent write cannot
// race the optimistic version check. The bool reports whether a row exists.
func lockNote(ctx context.Context, tx pgx.Tx, id string) (*NoteState, bool, error) {
	var st NoteState
	var supersedes *string
	err := tx.QueryRow(ctx,
		`SELECT id, kind, tier, title, body, supersedes, version, deleted
		   FROM notes
		  WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1
		  FOR UPDATE`, id).
		Scan(&st.ID, &st.Kind, &st.Tier, &st.Title, &st.Body, &supersedes, &st.Version, &st.Deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking note: %w", err)
	}
	if supersedes != nil {
		st.Supersedes = *supersedes
	}
	return &st, true, nil
}

// resolveWrite validates the optimistic precondition for a create/update and
// returns the op and the new version number. cur is nil when no row exists.
func resolveWrite(id string, expected int, cur *NoteState, found bool) (op string, newVersion int, err error) {
	if !found {
		if expected != 0 {
			return "", 0, &VersionConflictError{NoteID: id, Expected: expected, Actual: 0}
		}
		return "create", 1, nil
	}
	if cur.Version != expected {
		return "", 0, &VersionConflictError{NoteID: id, Expected: expected, Actual: cur.Version}
	}
	return "update", cur.Version + 1, nil
}

// upsertPGNote writes the live notes row for a create/update: it sets the new
// version, clears any tombstone, and records the writing surface.
// namespace/kind/tier are pre-normalized by the caller.
func upsertPGNote(ctx context.Context, tx pgx.Tx, in *PGNoteInput, namespace, kind, tier string, version int) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO notes (id, tenant_id, namespace, kind, tier, title, body, supersedes, version, deleted, updated_by)
		 VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, FALSE, $9)
		 ON CONFLICT (tenant_id, id)
		 DO UPDATE SET namespace = EXCLUDED.namespace, kind = EXCLUDED.kind, tier = EXCLUDED.tier,
		               title = EXCLUDED.title, body = EXCLUDED.body, supersedes = EXCLUDED.supersedes,
		               version = EXCLUDED.version, deleted = FALSE,
		               updated_by = EXCLUDED.updated_by, superseded = FALSE`,
		in.ID, namespace, kind, tier, in.Title, in.Body, in.Supersedes, version, in.Surface)
	if err != nil {
		return fmt.Errorf("upserting pg-native note: %w", err)
	}
	return nil
}

// appendVersion writes one immutable row to a note's history. The note_versions
// table is append-only (a database trigger blocks UPDATE/DELETE). surface is the
// audit trail — which client/identity made the write — and is always recorded;
// an empty one falls back to "unknown" rather than NULL so the log never has a
// gap.
func appendVersion(ctx context.Context, tx pgx.Tx, id string, version int, title, body, kind, tier, op, surface string) error {
	if surface == "" {
		surface = "unknown"
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO note_versions (tenant_id, note_id, version, title, body, kind, tier, op, surface)
		 VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8)`,
		id, version, title, body, kind, tier, op, surface)
	if err != nil {
		return fmt.Errorf("appending note version: %w", err)
	}
	return nil
}

// replaceChunks rewrites a note's chunk projection: delete then batch-insert.
// Passing no chunks just clears them (used when tombstoning a note).
func replaceChunks(ctx context.Context, tx pgx.Tx, noteID string, chunks []string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM chunks
		   WHERE tenant_id = current_setting('app.tenant_id')::uuid AND note_id = $1`, noteID); err != nil {
		return fmt.Errorf("clearing chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for ord, text := range chunks {
		batch.Queue(
			`INSERT INTO chunks (note_id, tenant_id, ord, text)
			 VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3)`,
			noteID, ord, text)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("inserting chunks: %w", err)
	}
	return nil
}

const selectVersionCols = `SELECT note_id, version, title, body, kind, tier, op, surface, created_at FROM note_versions`

// loadVersion fetches a history snapshot. A version of 0 selects the most recent
// non-delete version, i.e. the content to restore after a delete.
func loadVersion(ctx context.Context, tx pgx.Tx, id string, version int) (NoteVersion, error) {
	var row pgx.Row
	if version > 0 {
		row = tx.QueryRow(ctx,
			selectVersionCols+`
			 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND note_id = $1 AND version = $2`,
			id, version)
	} else {
		row = tx.QueryRow(ctx,
			selectVersionCols+`
			 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND note_id = $1 AND op <> 'delete'
			 ORDER BY version DESC LIMIT 1`,
			id)
	}
	nv, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return NoteVersion{}, &NoteNotFoundError{NoteID: id}
	}
	if err != nil {
		return NoteVersion{}, fmt.Errorf("loading note version: %w", err)
	}
	return nv, nil
}
