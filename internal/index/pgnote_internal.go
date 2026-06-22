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

// writeNoteTx performs a create/update inside an open transaction: lock + version
// check, supersedes-target existence check, the note/version/chunk writes, and a
// scoped supersession reconcile — all committing together. namespace/kind/tier
// are pre-normalized by the caller.
func writeNoteTx(ctx context.Context, tx pgx.Tx, in *PGNoteInput, namespace, kind, tier string, expectedVersion int) (WriteResult, error) {
	cur, found, err := lockNote(ctx, tx, in.ID)
	if err != nil {
		return WriteResult{}, err
	}
	op, newVersion, err := resolveWrite(in.ID, expectedVersion, cur, found)
	if err != nil {
		return WriteResult{}, err
	}
	// Existence of the supersedes target, checked in this same transaction so the
	// precondition and the write commit together (no check-then-write TOCTOU). An
	// empty pointer skips the check.
	if err := assertSupersedesTarget(ctx, tx, in.ID, in.Supersedes); err != nil {
		return WriteResult{}, err
	}
	var oldTarget string
	if found && cur != nil {
		oldTarget = cur.Supersedes
	}
	if err := upsertPGNote(ctx, tx, in, namespace, kind, tier, newVersion); err != nil {
		return WriteResult{}, err
	}
	if err := appendVersion(ctx, tx, in.ID, newVersion, in.Title, in.Body, kind, tier, op, in.Surface); err != nil {
		return WriteResult{}, err
	}
	if err := replaceChunks(ctx, tx, in.ID, Chunk(in.Title, in.Body, DefaultChunkWords)); err != nil {
		return WriteResult{}, err
	}
	// Reconcile only the notes this write can affect — the note itself and its
	// old/new supersedes targets — in the same tx, so there is no window where the
	// row is written but supersession flags are stale.
	n, err := reconcileSupersessionsTx(ctx, tx, in.ID, in.Supersedes, oldTarget)
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{ID: in.ID, Version: newVersion, Op: op, Superseded: n}, nil
}

// assertSupersedesTarget verifies, inside the write transaction, that a non-empty
// supersedes pointer targets a live (non-tombstoned) note. Running it in the same
// tx as the write closes the check-then-write race that a separate pre-check
// leaves open. An empty target is a no-op.
func assertSupersedesTarget(ctx context.Context, tx pgx.Tx, noteID, target string) error {
	if target == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM notes
		    WHERE tenant_id = current_setting('app.tenant_id')::uuid
		      AND id = $1 AND deleted = FALSE)`, target).Scan(&exists); err != nil {
		return fmt.Errorf("checking supersedes target: %w", err)
	}
	if !exists {
		return &SupersedesMissingError{NoteID: noteID, Target: target}
	}
	return nil
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
