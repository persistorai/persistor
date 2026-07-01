package index

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// NoteHit is one retrieved note, ranked by its best-matching chunk. Snippet is
// a short matched-fragment excerpt from that chunk, so the caller can judge
// relevance without a memory_get round-trip per candidate.
type NoteHit struct {
	ID         string
	Title      string
	Kind       string
	Tier       string
	Rank       float64
	Superseded bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Snippet    string
}

// maxSearchResults is a defensive upper bound on a search LIMIT. The MCP engine
// caps the caller-supplied limit already; this bounds every other caller (CLI,
// eval, future tools) so no single query can force an unbounded scan/sort.
const maxSearchResults = 500

// SearchOpts tunes a retrieval.
type SearchOpts struct {
	Limit             int
	IncludeSuperseded bool   // default retrieval excludes superseded notes
	Tier              string // "" = any; "core"/"tail" restrict (models the static baseline)
	Namespace         string // "" = all namespaces; otherwise restrict to one
	// Since/Until bound results by updated_at (nil = unbounded), enabling
	// temporal queries ("what did we decide last week") in SQL rather than
	// making the caller page and filter.
	Since *time.Time
	Until *time.Time
	// Kind restricts to one note kind (fact|decision|episode|reference|
	// preference); "" = any.
	Kind string
}

// SearchNotes runs full-text search over chunk tsvectors, deduplicates to the
// owning notes, and ranks each note by its best chunk. This is the primary
// retrieval path: deterministic, debuggable, no embeddings.
//
// Ranking is relevance first, recency as the TIEBREAKER (equal-rank notes
// surface newest-first — short personal-memory notes tie on ts_rank often, and
// newest-first is the right prior there, e.g. the latest status note for a
// project). Deliberately NOT a time-decay multiplier: in long-term memory age
// is not irrelevance — a strong match from years ago (the founding decision,
// the canonical fact) must outrank a weak fresh one, and STALENESS is already
// handled explicitly by supersession rather than guessed at by decay.
func (s *Store) SearchNotes(ctx context.Context, tenantID, query string, opts *SearchOpts) ([]NoteHit, error) {
	ctx, cancel := context.WithTimeout(ctx, storeQueryTimeout)
	defer cancel()

	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}
	limit = min(limit, maxSearchResults)

	// OR the query terms rather than AND-ing them. websearch_to_tsquery defaults
	// to AND, which makes recall brittle to a single extra/absent word; ORing the
	// content words and ranking by ts_rank lets notes that match most terms rise
	// to the top. Stopwords are dropped by the
	// 'english' config inside websearch_to_tsquery.
	orQuery := toOrQuery(query)

	// Inner query: rank each note by its best-matching chunk (DISTINCT ON keeps
	// that chunk's text), filter, order, LIMIT. Outer query: compute the
	// ts_headline excerpt only for the returned page — headline generation is
	// the expensive part and must not run across the whole match set.
	const q = `
		SELECT top.id, top.title, top.kind, top.tier, top.superseded, top.created_at, top.updated_at,
		       ts_headline('english', top.text, websearch_to_tsquery('english', $1),
		                   'MaxWords=30, MinWords=10, MaxFragments=2, FragmentDelimiter= … ') AS snippet,
		       top.rank
		FROM (
			SELECT best.*
			FROM (
				SELECT DISTINCT ON (n.id)
				       n.id, n.title, n.kind, n.tier, n.superseded, n.created_at, n.updated_at,
				       c.text, ts_rank(c.search_tsv, wq) AS rank
				FROM chunks c
				JOIN notes n
				  ON n.tenant_id = c.tenant_id AND n.id = c.note_id,
				     websearch_to_tsquery('english', $1) wq
				WHERE c.tenant_id = current_setting('app.tenant_id')::uuid
				  AND c.search_tsv @@ wq
				  AND n.deleted = FALSE
				  AND (n.superseded = FALSE OR $2)
				  AND ($3 = '' OR n.tier = $3)
				  AND ($4 = '' OR n.namespace = $4)
				  AND ($5::timestamptz IS NULL OR n.updated_at >= $5)
				  AND ($6::timestamptz IS NULL OR n.updated_at <= $6)
				  AND ($7 = '' OR n.kind = $7)
				ORDER BY n.id, ts_rank(c.search_tsv, wq) DESC
			) best
			ORDER BY best.rank DESC, best.updated_at DESC, best.id
			LIMIT $8
		) top
		ORDER BY top.rank DESC, top.updated_at DESC, top.id`

	var hits []NoteHit
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, orQuery, opts.IncludeSuperseded, opts.Tier, opts.Namespace, opts.Since, opts.Until, opts.Kind, limit)
		if err != nil {
			return fmt.Errorf("querying chunks: %w", err)
		}
		defer rows.Close()
		hits, err = scanNoteHits(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 && strings.TrimSpace(query) != "" {
		return s.searchFuzzy(ctx, tenantID, query, opts, limit)
	}
	return hits, nil
}

// searchFuzzy is the zero-hit fallback: trigram word-similarity over chunk
// text, catching what exact FTS lexemes miss — typos and misspelled entity
// names ("balast systm", "Meridain Acord"). It runs ONLY when FTS found
// nothing, so its similarity scores never mix with ts_rank scores. The <%
// operator's built-in word_similarity threshold (default 0.6) keeps this from
// dredging up junk for genuinely off-corpus queries — the abstention eval
// category guards that property.
func (s *Store) searchFuzzy(ctx context.Context, tenantID, query string, opts *SearchOpts, limit int) ([]NoteHit, error) {
	const q = `
		SELECT best.id, best.title, best.kind, best.tier, best.superseded, best.created_at, best.updated_at,
		       left(best.text, 200) AS snippet, best.rank
		FROM (
			SELECT DISTINCT ON (n.id)
			       n.id, n.title, n.kind, n.tier, n.superseded, n.created_at, n.updated_at,
			       c.text, word_similarity($1, c.text) AS rank
			FROM chunks c
			JOIN notes n
			  ON n.tenant_id = c.tenant_id AND n.id = c.note_id
			WHERE c.tenant_id = current_setting('app.tenant_id')::uuid
			  AND $1 <% c.text
			  AND n.deleted = FALSE
			  AND (n.superseded = FALSE OR $2)
			  AND ($3 = '' OR n.tier = $3)
			  AND ($4 = '' OR n.namespace = $4)
			  AND ($5::timestamptz IS NULL OR n.updated_at >= $5)
			  AND ($6::timestamptz IS NULL OR n.updated_at <= $6)
			  AND ($7 = '' OR n.kind = $7)
			ORDER BY n.id, word_similarity($1, c.text) DESC
		) best
		ORDER BY best.rank DESC, best.updated_at DESC, best.id
		LIMIT $8`

	var hits []NoteHit
	err := s.inReadTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, query, opts.IncludeSuperseded, opts.Tier, opts.Namespace, opts.Since, opts.Until, opts.Kind, limit)
		if err != nil {
			return fmt.Errorf("querying chunks (fuzzy): %w", err)
		}
		defer rows.Close()
		hits, err = scanNoteHits(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// scanNoteHits reads NoteHit rows in the shared search column order.
func scanNoteHits(rows pgx.Rows) ([]NoteHit, error) {
	var hits []NoteHit
	for rows.Next() {
		var h NoteHit
		if err := rows.Scan(&h.ID, &h.Title, &h.Kind, &h.Tier, &h.Superseded, &h.CreatedAt, &h.UpdatedAt, &h.Snippet, &h.Rank); err != nil {
			return nil, fmt.Errorf("scanning hit: %w", err)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
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
