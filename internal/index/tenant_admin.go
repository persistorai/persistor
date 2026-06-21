package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// DeleteTenantResult reports what a tenant purge removed.
type DeleteTenantResult struct {
	Notes      int64
	Versions   int64
	Chunks     int64
	Identities int64
}

// DeleteTenant hard-deletes ALL of a tenant's data — the operator "delete my
// account" purge. It removes the live notes, their chunks, and the full
// append-only version history (permitted here, and only here, via the app.purge
// escape hatch), plus the tenant's identity mappings. Per the no-soft-delete
// doctrine this is irreversible; the CLI gates it behind an explicit
// confirmation.
//
// The memory tables are RLS-scoped, so their deletes run with app.tenant_id set;
// identities is an RLS-exempt admin table keyed by tenant_id, deleted directly.
// Everything runs in one transaction: a failure leaves the tenant intact.
func (s *Store) DeleteTenant(ctx context.Context, tenantID string) (DeleteTenantResult, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var res DeleteTenantResult
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		// Authorize the one legal exception to the note_versions append-only guard
		// for this transaction only (set_config(..., true) is tx-local).
		if _, err := tx.Exec(ctx, "SELECT set_config('app.purge', 'on', true)"); err != nil {
			return fmt.Errorf("enabling purge: %w", err)
		}

		var err error
		guc := "tenant_id = current_setting('app.tenant_id')::uuid"
		if res.Chunks, err = execCount(ctx, tx, "DELETE FROM chunks WHERE "+guc); err != nil {
			return fmt.Errorf("deleting chunks: %w", err)
		}
		if res.Versions, err = execCount(ctx, tx, "DELETE FROM note_versions WHERE "+guc); err != nil {
			return fmt.Errorf("deleting note_versions: %w", err)
		}
		if res.Notes, err = execCount(ctx, tx, "DELETE FROM notes WHERE "+guc); err != nil {
			return fmt.Errorf("deleting notes: %w", err)
		}
		// RLS-exempt admin table: delete by explicit tenant_id.
		if res.Identities, err = execCount(ctx, tx, "DELETE FROM identities WHERE tenant_id = $1", tenantID); err != nil {
			return fmt.Errorf("deleting identities: %w", err)
		}
		return nil
	})
	if err != nil {
		return DeleteTenantResult{}, err
	}
	return res, nil
}

// PurgeNoteResult reports what a single-note purge removed.
type PurgeNoteResult struct {
	Found    bool
	Versions int64
	Chunks   int64
}

// PurgeNote irreversibly hard-deletes ONE note by id — its live row, its full
// append-only version history, and its chunk projection. It is the surgical
// counterpart to DeleteTenant: the operator "scrub this one note out of
// existence, history included" path, for a note that should never have been
// written. Per the no-soft-delete doctrine this is irreversible; the everyday,
// reversible path is a tombstone (memory_delete / DeleteNote), and the CLI gates
// this behind an explicit --yes.
//
// It runs with app.tenant_id set, so RLS scopes every delete to the tenant, and
// authorizes the note_versions append-only guard for this transaction only via
// the tx-local app.purge escape hatch. Found reports whether the id matched
// anything; purging an unknown id is a harmless no-op.
func (s *Store) PurgeNote(ctx context.Context, tenantID, noteID string) (PurgeNoteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var res PurgeNoteResult
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		// Authorize the one legal exception to the note_versions append-only guard
		// for this transaction only (set_config(..., true) is tx-local).
		if _, err := tx.Exec(ctx, "SELECT set_config('app.purge', 'on', true)"); err != nil {
			return fmt.Errorf("enabling purge: %w", err)
		}

		var err error
		guc := "tenant_id = current_setting('app.tenant_id')::uuid"
		if res.Chunks, err = execCount(ctx, tx, "DELETE FROM chunks WHERE "+guc+" AND note_id = $1", noteID); err != nil {
			return fmt.Errorf("deleting chunks: %w", err)
		}
		if res.Versions, err = execCount(ctx, tx, "DELETE FROM note_versions WHERE "+guc+" AND note_id = $1", noteID); err != nil {
			return fmt.Errorf("deleting note_versions: %w", err)
		}
		notes, err := execCount(ctx, tx, "DELETE FROM notes WHERE "+guc+" AND id = $1", noteID)
		if err != nil {
			return fmt.Errorf("deleting note: %w", err)
		}
		res.Found = notes > 0 || res.Versions > 0
		return nil
	})
	if err != nil {
		return PurgeNoteResult{}, err
	}
	return res, nil
}

// execCount runs a statement and returns the number of rows it affected.
func execCount(ctx context.Context, tx pgx.Tx, sql string, args ...any) (int64, error) {
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
