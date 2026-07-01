package index

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// NoteVersion is one immutable entry in a PG-native note's history.
type NoteVersion struct {
	NoteID    string
	Version   int
	Title     string
	Body      string
	Kind      string
	Tier      string
	Op        string
	Surface   string
	CreatedAt time.Time
}

// NoteVersions returns a note's full history, oldest version first.
func (s *Store) NoteVersions(ctx context.Context, tenantID, id string) ([]NoteVersion, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var out []NoteVersion
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			selectVersionCols+`
			 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND note_id = $1
			 ORDER BY version`, id)
		if err != nil {
			return fmt.Errorf("querying note versions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			nv, err := scanVersion(rows)
			if err != nil {
				return err
			}
			out = append(out, nv)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NoteState returns the current stored state of a note, including tombstones,
// and whether it exists. A caller reads it to obtain the version before an
// optimistic write.
func (s *Store) NoteState(ctx context.Context, tenantID, id string) (NoteState, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	var st NoteState
	found := false
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		var supersedes *string
		err := tx.QueryRow(ctx,
			`SELECT id, namespace, kind, tier, title, body, supersedes, version, deleted, created_at, updated_at
			   FROM notes
			  WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`, id).
			Scan(&st.ID, &st.Namespace, &st.Kind, &st.Tier, &st.Title, &st.Body, &supersedes, &st.Version, &st.Deleted,
				&st.CreatedAt, &st.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying note state: %w", err)
		}
		if supersedes != nil {
			st.Supersedes = *supersedes
		}
		found = true
		return nil
	})
	if err != nil {
		return NoteState{}, false, err
	}
	return st, found, nil
}

// scanVersion reads one note_versions row, mapping a NULL surface to "". It
// accepts a pgx.Row; pgx.Rows satisfies that interface, so both the single-row
// and multi-row read paths share it.
func scanVersion(row pgx.Row) (NoteVersion, error) {
	var nv NoteVersion
	var surface *string
	if err := row.Scan(&nv.NoteID, &nv.Version, &nv.Title, &nv.Body, &nv.Kind, &nv.Tier, &nv.Op, &surface, &nv.CreatedAt); err != nil {
		return NoteVersion{}, err
	}
	if surface != nil {
		nv.Surface = *surface
	}
	return nv, nil
}
