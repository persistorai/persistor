package index

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// defaultNamespace is the namespace a note lands in when the writer does not
// specify one — the per-tenant catch-all bucket.
const defaultNamespace = "default"

// PGNoteInput is a PG-native note write: the note row itself is the source of
// truth (no backing file), so the write goes straight to Postgres with an
// append-only version history. Empty Kind/Tier default to "fact"/"tail"; empty
// Namespace defaults to "default".
type PGNoteInput struct {
	ID         string
	Namespace  string
	Kind       string
	Tier       string
	Title      string
	Body       string
	Supersedes string
	Surface    string // client/surface that made the write; recorded for audit.
}

// WriteResult reports the outcome of a PG-native mutation.
type WriteResult struct {
	ID      string
	Version int
	Op      string // create | update | delete | restore.
}

// NoteState is the current stored state of a PG-native note, including the
// tombstone flag and version. A caller reads it before an optimistic write.
type NoteState struct {
	ID         string
	Namespace  string
	Kind       string
	Tier       string
	Title      string
	Body       string
	Supersedes string
	Version    int
	Deleted    bool
}

// VersionConflictError is returned when an optimistic write's expected version
// does not match the stored version. The caller should re-read and retry; the
// remote MCP layer maps this to HTTP 409.
type VersionConflictError struct {
	NoteID   string
	Expected int
	Actual   int
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("version conflict on note %q: expected %d, have %d", e.NoteID, e.Expected, e.Actual)
}

// NoteNotFoundError is returned when an operation requires a live note row that
// is absent or tombstoned.
type NoteNotFoundError struct{ NoteID string }

func (e *NoteNotFoundError) Error() string {
	return fmt.Sprintf("note %q not found", e.NoteID)
}

// WriteNote creates or updates a PG-native note under optimistic concurrency.
// expectedVersion is 0 for a create (no live or tombstoned row may exist for the
// id) and the current version for an update; a mismatch returns
// *VersionConflictError. Writing over a tombstone un-deletes it. The note row,
// its appended history entry, and its chunk projection are written in one
// transaction.
func (s *Store) WriteNote(ctx context.Context, tenantID string, in *PGNoteInput, expectedVersion int) (WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	kind, tier, namespace := in.Kind, in.Tier, in.Namespace
	if kind == "" {
		kind = "fact"
	}
	if tier == "" {
		tier = "tail"
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	var res WriteResult
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		cur, found, err := lockNote(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		op, newVersion, err := resolveWrite(in.ID, expectedVersion, cur, found)
		if err != nil {
			return err
		}
		if err := upsertPGNote(ctx, tx, in, namespace, kind, tier, newVersion); err != nil {
			return err
		}
		if err := appendVersion(ctx, tx, in.ID, newVersion, in.Title, in.Body, kind, tier, op, in.Surface); err != nil {
			return err
		}
		if err := replaceChunks(ctx, tx, in.ID, Chunk(in.Title, in.Body, DefaultChunkWords)); err != nil {
			return err
		}
		res = WriteResult{ID: in.ID, Version: newVersion, Op: op}
		return nil
	})
	if err != nil {
		// A create (expectedVersion 0) can't see a concurrent creator: both lock
		// nothing, both resolve to version 1, and the loser only trips the
		// note_versions PK (tenant_id, note_id, version) as a unique violation.
		// The row now exists at version 1, so report it as the conflict it is —
		// a clean *VersionConflictError (409), not the raw unique-violation (500).
		if expectedVersion == 0 && isUniqueViolation(err) {
			return WriteResult{}, &VersionConflictError{NoteID: in.ID, Expected: 0, Actual: 1}
		}
		return WriteResult{}, err
	}
	return res, nil
}

// isUniqueViolation reports whether err is (or wraps) a PostgreSQL
// unique_violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ReindexPGNative rebuilds the chunk projection for every live PG-native note
// (empty source_path, not tombstoned) from its current body — the PG-to-PG
// equivalent of the file-sync reindexer. File-backed notes are left to the disk
// indexer. It returns the number of notes re-chunked.
func (s *Store) ReindexPGNative(ctx context.Context, tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	recs, err := s.pgNativeNotes(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	err = s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		for _, r := range recs {
			if err := replaceChunks(ctx, tx, r.ID, Chunk(r.Title, r.Body, DefaultChunkWords)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(recs), nil
}

// pgNativeNotes reads the live PG-native notes (empty source_path, not deleted)
// for a tenant, ordered by id.
func (s *Store) pgNativeNotes(ctx context.Context, tenantID string) ([]NoteRecord, error) {
	var out []NoteRecord
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, kind, tier, title, body, source_path
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid
			    AND source_path = '' AND deleted = FALSE
			  ORDER BY id`)
		if err != nil {
			return fmt.Errorf("querying pg-native notes: %w", err)
		}
		defer rows.Close()
		out, err = scanNoteRecords(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteNote tombstones a live PG-native note: it sets deleted=true, appends a
// delete version, and drops the note's chunks so live search cannot surface it.
// The body snapshots in history remain, so the note is restorable. A missing or
// already-tombstoned note returns *NoteNotFoundError; a version mismatch returns
// *VersionConflictError.
func (s *Store) DeleteNote(ctx context.Context, tenantID, id string, expectedVersion int, surface string) (WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var res WriteResult
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		cur, found, err := lockNote(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found || cur.Deleted {
			return &NoteNotFoundError{NoteID: id}
		}
		if cur.Version != expectedVersion {
			return &VersionConflictError{NoteID: id, Expected: expectedVersion, Actual: cur.Version}
		}
		newVersion := cur.Version + 1
		if _, err := tx.Exec(ctx,
			`UPDATE notes SET deleted = TRUE, version = $2, updated_by = $3
			   WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
			id, newVersion, surface); err != nil {
			return fmt.Errorf("tombstoning note: %w", err)
		}
		if err := appendVersion(ctx, tx, id, newVersion, cur.Title, cur.Body, cur.Kind, cur.Tier, "delete", surface); err != nil {
			return err
		}
		if err := replaceChunks(ctx, tx, id, nil); err != nil {
			return err
		}
		res = WriteResult{ID: id, Version: newVersion, Op: "delete"}
		return nil
	})
	if err != nil {
		return WriteResult{}, err
	}
	return res, nil
}

// RestoreNote copies a prior version's content forward as a new version with
// deleted=false — the undo for an accidental delete or a bad overwrite. A
// targetVersion of 0 restores the most recent non-delete version. The current
// row must match expectedVersion. A missing note or version returns
// *NoteNotFoundError; a version mismatch returns *VersionConflictError.
func (s *Store) RestoreNote(ctx context.Context, tenantID, id string, targetVersion, expectedVersion int, surface string) (WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var res WriteResult
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		cur, found, err := lockNote(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			return &NoteNotFoundError{NoteID: id}
		}
		if cur.Version != expectedVersion {
			return &VersionConflictError{NoteID: id, Expected: expectedVersion, Actual: cur.Version}
		}
		snap, err := loadVersion(ctx, tx, id, targetVersion)
		if err != nil {
			return err
		}
		newVersion := cur.Version + 1
		if _, err := tx.Exec(ctx,
			`UPDATE notes SET title = $2, body = $3, kind = $4, tier = $5,
			        deleted = FALSE, version = $6, updated_by = $7, superseded = FALSE
			   WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`,
			id, snap.Title, snap.Body, snap.Kind, snap.Tier, newVersion, surface); err != nil {
			return fmt.Errorf("restoring note: %w", err)
		}
		if err := appendVersion(ctx, tx, id, newVersion, snap.Title, snap.Body, snap.Kind, snap.Tier, "restore", surface); err != nil {
			return err
		}
		if err := replaceChunks(ctx, tx, id, Chunk(snap.Title, snap.Body, DefaultChunkWords)); err != nil {
			return err
		}
		res = WriteResult{ID: id, Version: newVersion, Op: "restore"}
		return nil
	})
	if err != nil {
		return WriteResult{}, err
	}
	return res, nil
}
