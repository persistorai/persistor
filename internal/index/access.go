package index

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// BumpAccess records that the given notes were read (a memory_get, or surfaced
// on a search results page): one upsert incrementing access_count and stamping
// last_accessed_at. Telemetry only — it feeds ops queries and future design
// decisions (e.g. whether decay is ever justified) and is deliberately NOT a
// ranking input. Callers treat failures as non-fatal: losing a tick must never
// fail a read.
func (s *Store) BumpAccess(ctx context.Context, tenantID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	return s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO note_access (tenant_id, note_id, access_count, last_accessed_at)
			 SELECT current_setting('app.tenant_id')::uuid, unnest($1::text[]), 1, NOW()
			 ON CONFLICT (tenant_id, note_id)
			 DO UPDATE SET access_count = note_access.access_count + 1,
			               last_accessed_at = NOW()`, ids)
		return err
	})
}
