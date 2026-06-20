package index

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	// defaultListLimit is the page size used when a caller does not specify one.
	defaultListLimit = 50
	// maxListLimit is the hard cap on a single list page, so a tenant with many
	// notes can't pull the whole corpus in one unbounded query.
	maxListLimit = 500
)

// NoteSummary is one note in an enumeration: the metadata needed to browse or
// page a tenant's memory without pulling bodies. The body is fetched separately
// (LoadNotes / the memory_get tool) once a specific note is chosen.
type NoteSummary struct {
	ID         string
	Namespace  string
	Kind       string
	Tier       string
	Title      string
	Version    int
	Superseded bool
}

// ListOpts tunes an enumeration. Namespace "" lists every namespace; a
// non-empty Limit/Offset paginate; IncludeSuperseded adds corrected/stale notes
// (excluded by default, matching search).
type ListOpts struct {
	Namespace         string
	Limit             int
	Offset            int
	IncludeSuperseded bool
}

// NamespaceCount is one namespace and how many live notes it holds.
type NamespaceCount struct {
	Namespace string
	Count     int
}

// ListNotes enumerates a tenant's live notes (newest schema order: by id) as
// summaries, optionally filtered to one namespace and paginated. Tombstoned
// notes are always excluded; superseded notes are excluded unless
// opts.IncludeSuperseded. Limit defaults to defaultListLimit and is capped at
// maxListLimit; a negative offset is treated as 0. This is the enumerate path
// the search index cannot serve (search needs a query term).
func (s *Store) ListNotes(ctx context.Context, tenantID string, opts ListOpts) ([]NoteSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	limit = min(limit, maxListLimit)
	offset := max(opts.Offset, 0)

	const q = `
		SELECT id, namespace, kind, tier, title, version, superseded
		  FROM notes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND deleted = FALSE
		   AND ($1 = '' OR namespace = $1)
		   AND (superseded = FALSE OR $2)
		 ORDER BY id
		 LIMIT $3 OFFSET $4`

	var out []NoteSummary
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, opts.Namespace, opts.IncludeSuperseded, limit, offset)
		if err != nil {
			return fmt.Errorf("listing notes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var n NoteSummary
			if err := rows.Scan(&n.ID, &n.Namespace, &n.Kind, &n.Tier, &n.Title, &n.Version, &n.Superseded); err != nil {
				return fmt.Errorf("scanning summary: %w", err)
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

// Namespaces returns every namespace the tenant has live (non-tombstoned) notes
// in, each with its note count, ordered by namespace. Superseded notes are
// counted — they are still live rows the tenant owns.
func (s *Store) Namespaces(ctx context.Context, tenantID string) ([]NamespaceCount, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	const q = `
		SELECT namespace, count(*)
		  FROM notes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid
		   AND deleted = FALSE
		 GROUP BY namespace
		 ORDER BY namespace`

	var out []NamespaceCount
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q)
		if err != nil {
			return fmt.Errorf("listing namespaces: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var nc NamespaceCount
			if err := rows.Scan(&nc.Namespace, &nc.Count); err != nil {
				return fmt.Errorf("scanning namespace count: %w", err)
			}
			out = append(out, nc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
