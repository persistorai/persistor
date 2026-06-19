package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// NoteRecord is a full note row read back from the index (title + body), used to
// assemble the working-set. Distinct from NoteHit, which is a ranked
// search result without the body.
type NoteRecord struct {
	ID         string
	Kind       string
	Tier       string
	Title      string
	Body       string
	SourcePath string
}

// CoreNotes returns every Core-tier note (id, title, body) for the tenant,
// ordered deterministically by id. These are the always-loaded surface.
func (s *Store) CoreNotes(ctx context.Context, tenantID string) ([]NoteRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var out []NoteRecord
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, kind, tier, title, body, source_path
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid
			    AND tier = 'core' AND superseded = FALSE AND deleted = FALSE
			  ORDER BY id`)
		if err != nil {
			return fmt.Errorf("querying core notes: %w", err)
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

// LoadNotes fetches full note records for the given ids, preserving the input
// order (so a ranked list of ids stays ranked). Missing ids are skipped.
func (s *Store) LoadNotes(ctx context.Context, tenantID string, ids []string) ([]NoteRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	byID := make(map[string]NoteRecord, len(ids))
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, kind, tier, title, body, source_path
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid
			    AND id = ANY($1) AND deleted = FALSE`, ids)
		if err != nil {
			return fmt.Errorf("querying notes: %w", err)
		}
		defer rows.Close()
		recs, err := scanNoteRecords(rows)
		if err != nil {
			return err
		}
		for _, r := range recs {
			byID[r.ID] = r
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	ordered := make([]NoteRecord, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			ordered = append(ordered, r)
		}
	}
	return ordered, nil
}

func scanNoteRecords(rows pgx.Rows) ([]NoteRecord, error) {
	var out []NoteRecord
	for rows.Next() {
		var r NoteRecord
		if err := rows.Scan(&r.ID, &r.Kind, &r.Tier, &r.Title, &r.Body, &r.SourcePath); err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
