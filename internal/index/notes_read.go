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
	ID    string
	Kind  string
	Tier  string
	Title string
	Body  string
}

// maxCoreNotes caps how many Core-tier notes the always-loaded surface fetches.
// Core is meant to be a small, curated pin set; this stops a tenant that writes
// thousands of core notes from making CoreNotes an unbounded fetch into the
// working set. The brief budgeter truncates further, so the cap only bounds the
// read.
const maxCoreNotes = 500

// CoreNotes returns the Core-tier notes (id, title, body) for the tenant, ordered
// deterministically by id and capped at maxCoreNotes. These are the always-loaded
// surface. A tenant at the cap is logged so an oversized Core tier is visible.
func (s *Store) CoreNotes(ctx context.Context, tenantID string) ([]NoteRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var out []NoteRecord
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, kind, tier, title, body
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid
			    AND tier = 'core' AND superseded = FALSE AND deleted = FALSE
			  ORDER BY id
			  LIMIT $1`, maxCoreNotes)
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
	if len(out) == maxCoreNotes && s.log != nil {
		s.log.WithField("cap", maxCoreNotes).
			Warn("Core tier hit the CoreNotes cap; some core notes are excluded from the working set")
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
			`SELECT id, kind, tier, title, body
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
		if err := rows.Scan(&r.ID, &r.Kind, &r.Tier, &r.Title, &r.Body); err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
