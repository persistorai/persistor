package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ExportNotes returns every current (non-deleted) note for a tenant, ordered by
// id — the read side of `persistor export`. It runs under RLS with app.tenant_id
// set to tenantID, so a caller can never read across tenants. Tombstones are
// excluded: an export is the live memory, not its history.
func (s *Store) ExportNotes(ctx context.Context, tenantID string) ([]Note, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var out []Note
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, kind, tier, title, body, source_path, COALESCE(supersedes, '')
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid AND deleted = FALSE
			  ORDER BY id`)
		if err != nil {
			return fmt.Errorf("querying notes for export: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var n Note
			if err := rows.Scan(&n.ID, &n.Kind, &n.Tier, &n.Title, &n.Body, &n.SourcePath, &n.Supersedes); err != nil {
				return fmt.Errorf("scanning note: %w", err)
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
