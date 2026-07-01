package index_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/briancolinger/persistor/internal/index"
)

// TestSearchAndListTimeBounds exercises the Since/Until updated_at filters and
// the timestamp fields, using the note's own stored time as the pivot (the
// update trigger forces updated_at = NOW(), so ages can't be fabricated).
func TestSearchAndListTimeBounds(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:timebound"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "chronology pivot walrus"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	st, found, err := store.NoteState(ctx, tenant, id)
	if err != nil || !found {
		t.Fatalf("state: found=%v err=%v", found, err)
	}
	if st.CreatedAt.IsZero() || st.UpdatedAt.IsZero() {
		t.Fatal("NoteState timestamps are zero")
	}
	pivot := st.UpdatedAt
	before := pivot.Add(-time.Hour)
	after := pivot.Add(time.Hour)

	search := func(since, until *time.Time) []index.NoteHit {
		t.Helper()
		hits, err := store.SearchNotes(ctx, tenant, "walrus", &index.SearchOpts{Limit: 5, Since: since, Until: until})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		return hits
	}

	if hits := search(nil, nil); len(hits) != 1 || hits[0].UpdatedAt.IsZero() || hits[0].CreatedAt.IsZero() {
		t.Fatalf("unbounded search: hits=%d (want 1 with timestamps)", len(hits))
	}
	if hits := search(&before, nil); len(hits) != 1 {
		t.Errorf("since<updated: hits=%d, want 1", len(hits))
	}
	if hits := search(&after, nil); len(hits) != 0 {
		t.Errorf("since>updated: hits=%d, want 0", len(hits))
	}
	if hits := search(nil, &after); len(hits) != 1 {
		t.Errorf("until>updated: hits=%d, want 1", len(hits))
	}
	if hits := search(nil, &before); len(hits) != 0 {
		t.Errorf("until<updated: hits=%d, want 0", len(hits))
	}

	// List honors the same bounds and carries timestamps.
	if got, err := store.ListNotes(ctx, tenant, index.ListOpts{Since: &before}); err != nil || len(got) != 1 || got[0].UpdatedAt.IsZero() {
		t.Errorf("list since<updated: n=%d err=%v (want 1 with timestamps)", len(got), err)
	}
	if got, err := store.ListNotes(ctx, tenant, index.ListOpts{Since: &after}); err != nil || len(got) != 0 {
		t.Errorf("list since>updated: n=%d err=%v, want 0", len(got), err)
	}
}

// TestSearchRecencyTiebreak: equal-relevance notes surface newest-first. Two
// notes with identical match text tie on ts_rank; the later-updated one must
// rank first, and relevance must still dominate over recency for unequal ranks.
func TestSearchRecencyTiebreak(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	// Identical bodies -> identical ts_rank. B is written (updated) after A.
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: "test:tie-a", Body: "quarterly narwhal census results"}, 0); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: "test:tie-b", Body: "quarterly narwhal census results"}, 0); err != nil {
		t.Fatalf("create b: %v", err)
	}

	hits, err := store.SearchNotes(ctx, tenant, "narwhal census", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].ID != "test:tie-b" || hits[1].ID != "test:tie-a" {
		t.Errorf("tie order = [%s, %s], want newest-first [test:tie-b, test:tie-a]", hits[0].ID, hits[1].ID)
	}
	if hits[0].Rank != hits[1].Rank {
		t.Fatalf("test premise broken: ranks differ (%v vs %v), not a tie", hits[0].Rank, hits[1].Rank)
	}

	// Relevance still dominates: an older note that matches BOTH terms in its
	// title+body outranks the newer single-topic ties for a two-term query.
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: "test:tie-strong", Title: "Narwhal census methodology",
		Body: "How the narwhal census is run: count narwhal pods during the census window.",
	}, 0); err != nil {
		t.Fatalf("create strong: %v", err)
	}
	// Freshen the weak ties so the strong note is the OLDEST of the three.
	for _, id := range []string{"test:tie-a", "test:tie-b"} {
		st, _, err := store.NoteState(ctx, tenant, id)
		if err != nil {
			t.Fatalf("state %s: %v", id, err)
		}
		if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: st.Body + " refreshed"}, st.Version); err != nil {
			t.Fatalf("touch %s: %v", id, err)
		}
	}
	hits, err = store.SearchNotes(ctx, tenant, "narwhal census", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search 2: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "test:tie-strong" {
		got := "none"
		if len(hits) > 0 {
			got = hits[0].ID
		}
		t.Errorf("first hit = %s, want the higher-relevance (but older) test:tie-strong", got)
	}
}

// TestSearchSnippetsAndKindFilter: hits carry a matched-fragment snippet
// (ts_headline on the FTS path, a plain prefix on the fuzzy path) and the kind
// filter restricts both search and list.
func TestSearchSnippetsAndKindFilter(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: "test:snip-fact", Kind: "fact",
		Body: "The wolverine enclosure gate code was changed after the escape incident last spring.",
	}, 0); err != nil {
		t.Fatalf("create fact: %v", err)
	}
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: "test:snip-decision", Kind: "decision",
		Body: "Decision: the wolverine enclosure will be rebuilt with double gates.",
	}, 0); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	hits, err := store.SearchNotes(ctx, tenant, "wolverine enclosure gate", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	for i := range hits {
		if hits[i].Snippet == "" {
			t.Errorf("hit %s has no snippet", hits[i].ID)
		}
		if !strings.Contains(hits[i].Snippet, "<b>") {
			t.Errorf("hit %s snippet lacks ts_headline match markers: %q", hits[i].ID, hits[i].Snippet)
		}
	}

	// Kind filter narrows the same query to the decision note only.
	hits, err = store.SearchNotes(ctx, tenant, "wolverine enclosure gate", &index.SearchOpts{Limit: 5, Kind: "decision"})
	if err != nil {
		t.Fatalf("search kind: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "test:snip-decision" {
		t.Fatalf("kind-filtered hits = %v, want just test:snip-decision", hits)
	}

	// Fuzzy fallback (typo query) also carries a snippet.
	hits, err = store.SearchNotes(ctx, tenant, "wolveriene enclosur gate", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("fuzzy search: %v", err)
	}
	if len(hits) == 0 || hits[0].Snippet == "" {
		t.Fatalf("fuzzy hits = %d, want >0 with snippets", len(hits))
	}

	// List honors the kind filter.
	sums, err := store.ListNotes(ctx, tenant, index.ListOpts{Kind: "decision"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for i := range sums {
		if sums[i].Kind != "decision" {
			t.Errorf("list leaked kind %q (id %s)", sums[i].Kind, sums[i].ID)
		}
	}
}

// TestBumpAccess: telemetry upserts increment access_count without touching
// the note row (updated_at must NOT move — it feeds the F2 recency tiebreak).
func TestBumpAccess(t *testing.T) {
	store, pool, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:access"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "telemetry ocelot"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	before, _, err := store.NoteState(ctx, tenant, id)
	if err != nil {
		t.Fatalf("state: %v", err)
	}

	if err := store.BumpAccess(ctx, tenant, []string{id}); err != nil {
		t.Fatalf("bump 1: %v", err)
	}
	if err := store.BumpAccess(ctx, tenant, []string{id}); err != nil {
		t.Fatalf("bump 2: %v", err)
	}

	var count int64
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	if err := tx.QueryRow(ctx,
		"SELECT access_count FROM note_access WHERE note_id = $1", id).Scan(&count); err != nil {
		t.Fatalf("read count: %v", err)
	}
	if count != 2 {
		t.Errorf("access_count = %d, want 2", count)
	}

	after, _, err := store.NoteState(ctx, tenant, id)
	if err != nil {
		t.Fatalf("state after: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("updated_at moved (%v -> %v); telemetry must not touch the note row", before.UpdatedAt, after.UpdatedAt)
	}
}
