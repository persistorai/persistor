package index

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// NoteHit is one retrieved note, ranked by its best-matching chunk.
type NoteHit struct {
	ID         string
	Title      string
	Kind       string
	Tier       string
	Rank       float64
	Superseded bool
}

// SearchOpts tunes a retrieval.
type SearchOpts struct {
	Limit             int
	IncludeSuperseded bool   // default retrieval excludes superseded notes
	Tier              string // "" = any; "core"/"tail" restrict (models the static baseline)
}

// SearchNotes runs full-text search over chunk tsvectors, deduplicates to the
// owning notes, and ranks each note by its best chunk. This is the// primary retrieval path: deterministic, debuggable, no embeddings.
func (s *Store) SearchNotes(ctx context.Context, tenantID, query string, opts SearchOpts) ([]NoteHit, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	// OR the query terms rather than AND-ing them. websearch_to_tsquery defaults
	// to AND, which makes recall brittle to a single extra/absent word; ORing the
	// content words and ranking by ts_rank lets notes that match most terms rise
	// to the top. Stopwords are dropped by the
	// 'english' config inside websearch_to_tsquery.
	orQuery := toOrQuery(query)

	const q = `
		SELECT n.id, n.title, n.kind, n.tier, n.superseded, max(ts_rank(c.search_tsv, wq)) AS rank
		FROM chunks c
		JOIN notes n
		  ON n.tenant_id = c.tenant_id AND n.id = c.note_id,
		     websearch_to_tsquery('english', $1) wq
		WHERE c.tenant_id = current_setting('app.tenant_id')::uuid
		  AND c.search_tsv @@ wq
		  AND (n.superseded = FALSE OR $2)
		  AND ($3 = '' OR n.tier = $3)
		GROUP BY n.id, n.title, n.kind, n.tier, n.superseded
		ORDER BY rank DESC, n.id
		LIMIT $4`

	var hits []NoteHit
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, orQuery, opts.IncludeSuperseded, opts.Tier, limit)
		if err != nil {
			return fmt.Errorf("querying chunks: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h NoteHit
			if err := rows.Scan(&h.ID, &h.Title, &h.Kind, &h.Tier, &h.Superseded, &h.Rank); err != nil {
				return fmt.Errorf("scanning hit: %w", err)
			}
			hits = append(hits, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// toOrQuery turns a free-text query into a websearch "a OR b OR c" expression
// over its word tokens, so any matching term contributes (recall) and ts_rank
// orders by how many/how strongly the terms match (precision). Returns "" for
// an all-punctuation/empty query, which matches nothing.
func toOrQuery(query string) string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			terms = append(terms, f)
		}
	}
	return strings.Join(terms, " OR ")
}
