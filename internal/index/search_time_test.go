package index_test

import (
	"context"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/index"
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
		hits, err := store.SearchNotes(ctx, tenant, "walrus", index.SearchOpts{Limit: 5, Since: since, Until: until})
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
