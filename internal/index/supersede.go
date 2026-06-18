package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ReconcileSupersessions recomputes the superseded flag for every note in the
// tenant from the current set of `supersedes` pointers. A note is
// superseded iff some other note points at it via supersedes. This is derived
// state, recomputed in one pass after a reindex, so it is order-independent and
// self-healing: it does not matter whether the superseding note or its target
// was indexed first, and deleting a superseding note resurrects its target.
//
// Only rows whose flag actually changes are written, so updated_at (and the
// update_timestamp trigger) is not churned on every reindex.
func (s *Store) ReconcileSupersessions(ctx context.Context, tenantID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	// The subquery is the set of ids that are currently superseded: any id that
	// appears as another note's supersedes target (excluding a note pointing at
	// itself, which is meaningless). The outer WHERE limits the UPDATE to rows
	// whose stored flag disagrees with the derived value.
	const q = `
		UPDATE notes
		   SET superseded = (id IN (
		         SELECT supersedes FROM notes
		          WHERE tenant_id = current_setting('app.tenant_id')::uuid
		            AND supersedes IS NOT NULL AND supersedes <> id))
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND superseded IS DISTINCT FROM (id IN (
		         SELECT supersedes FROM notes
		          WHERE tenant_id = current_setting('app.tenant_id')::uuid
		            AND supersedes IS NOT NULL AND supersedes <> id))`

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
