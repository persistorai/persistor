package index_test

import (
	"context"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

// TestPurgeNote verifies the operator single-note hard-purge: it removes a live
// note's row, its full append-only history, and its chunks, leaving sibling
// notes in the same tenant untouched; purging an unknown id is a harmless no-op.
func TestPurgeNote(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	const id = "test:purge-me"
	const sibling = "test:keep-me"

	// A live note with history (create -> update, not tombstoned, so it still has
	// chunks), plus a sibling that must survive the purge.
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "first body"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "second body"}, 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: sibling, Body: "untouched"}, 0); err != nil {
		t.Fatalf("sibling create: %v", err)
	}

	res, err := store.PurgeNote(ctx, tenant, id)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if !res.Found {
		t.Fatal("purge: Found=false, want true")
	}
	if res.Versions < 2 {
		t.Fatalf("purge: Versions=%d, want >=2 (create+update history)", res.Versions)
	}
	if res.Chunks < 1 {
		t.Fatalf("purge: Chunks=%d, want >=1 (a live note has chunks)", res.Chunks)
	}

	// The note is gone entirely.
	if _, found, err := store.NoteState(ctx, tenant, id); err != nil || found {
		t.Fatalf("note still present after purge: found=%v err=%v", found, err)
	}
	// Its history is gone too (purging it again finds nothing).
	again, err := store.PurgeNote(ctx, tenant, id)
	if err != nil {
		t.Fatalf("re-purge: %v", err)
	}
	if again.Found || again.Versions != 0 || again.Chunks != 0 {
		t.Fatalf("history survived purge: %+v", again)
	}

	// The sibling is untouched.
	if _, found, err := store.NoteState(ctx, tenant, sibling); err != nil || !found {
		t.Fatalf("sibling note vanished: found=%v err=%v", found, err)
	}

	// Purging an unknown id is a harmless no-op.
	unknown, err := store.PurgeNote(ctx, tenant, "test:does-not-exist")
	if err != nil {
		t.Fatalf("purge unknown: %v", err)
	}
	if unknown.Found {
		t.Fatal("purge unknown: Found=true, want false")
	}
}
