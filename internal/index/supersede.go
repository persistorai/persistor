package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// activeSupersedeTargets is the set of note ids that are currently superseded:
// any id another live note points at via supersedes (a note pointing at itself
// is meaningless and excluded; a tombstoned superseding note is ignored, so
// deleting a correction resurrects the note it superseded). With the partial
// index on notes(tenant_id, supersedes) WHERE supersedes IS NOT NULL this scans
// only the superseding rows, not the whole tenant.
const activeSupersedeTargets = `
	SELECT supersedes FROM notes
	 WHERE tenant_id = current_setting('app.tenant_id')::uuid
	   AND supersedes IS NOT NULL AND supersedes <> id AND deleted = FALSE`

// ReconcileSupersessions recomputes the superseded flag for every note in the
// tenant from the current set of `supersedes` pointers. A note is superseded iff
// some other live note points at it. This is the full-tenant pass, used after a
// bulk import where many pointers change at once; the per-write path uses the
// scoped reconcileSupersessionsTx instead. Order-independent and self-healing.
//
// Only rows whose flag actually changes are written, so updated_at (and the
// update_timestamp trigger) is not churned on every reconcile.
func (s *Store) ReconcileSupersessions(ctx context.Context, tenantID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	const q = `
		UPDATE notes
		   SET superseded = (id IN (` + activeSupersedeTargets + `))
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND superseded IS DISTINCT FROM (id IN (` + activeSupersedeTargets + `))`

	var affected int64
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q)
		if err != nil {
			return fmt.Errorf("reconciling supersessions: %w", err)
		}
		affected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// reconcileSupersessionsTx recomputes the superseded flag for only the given
// candidate ids, inside an existing write transaction. A single note mutation
// can only change the flag of the note itself, its old supersedes target, and
// its new supersedes target — so the per-write path scopes the reconcile to
// those ids instead of rescanning the whole tenant on every write/delete/
// restore. The derived membership is still computed from the live target set,
// so each candidate's flag is correct; only the rows considered for update are
// bounded. Empty/duplicate ids are dropped.
func reconcileSupersessionsTx(ctx context.Context, tx pgx.Tx, ids ...string) (int64, error) {
	cand := dedupeNonEmpty(ids)
	if len(cand) == 0 {
		return 0, nil
	}

	const q = `
		UPDATE notes
		   SET superseded = (id IN (` + activeSupersedeTargets + `))
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND id = ANY($1)
		   AND superseded IS DISTINCT FROM (id IN (` + activeSupersedeTargets + `))`

	tag, err := tx.Exec(ctx, q, cand)
	if err != nil {
		return 0, fmt.Errorf("reconciling supersessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// dedupeNonEmpty returns the distinct non-empty strings in ids, preserving first
// appearance order.
func dedupeNonEmpty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
