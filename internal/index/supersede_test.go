package index_test

import (
	"context"
	"errors"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

// TestReconcileSupersessions checks supersession: a superseding note hides the
// stale note from default retrieval, the stale note is still retrievable with
// IncludeSuperseded, and an unrelated time-bound note coexists (is never
// superseded). It also checks self-healing: deleting the superseding note
// resurrects the target.
func TestReconcileSupersessions(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	// Old fact, the current correction that supersedes it, and an independent
	// time-bound fact that must NOT be superseded (both true at their time).
	seedMarkdown(t, store, tenantID, "weight-old.md",
		"---\nid: syn:weight\ntitle: Weight\n---\n# Weight\n\nWeighs 250 pounds at the desk.\n", false)
	seedMarkdown(t, store, tenantID, "weight-new.md",
		"---\nid: syn:weight-2026\ntitle: Weight 2026\nsupersedes: syn:weight\n---\n# Weight 2026\n\nWeighs 230 pounds now.\n", false)
	seedMarkdown(t, store, tenantID, "weight-2022.md",
		"---\nid: syn:weight-2022\ntitle: Weight 2022\n---\n# Weight 2022\n\nWeighed 190 pounds in 2022 in Oklahoma.\n", false)

	// Supersession is reconciled at write time, inside WriteNote's transaction, so
	// seeding the correction already flipped syn:weight. An explicit full reconcile
	// is therefore idempotent here — it has nothing left to change.
	changed, err := store.ReconcileSupersessions(ctx, tenantID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if changed != 0 {
		t.Errorf("supersession should already be reconciled at write time; full reconcile changed %d", changed)
	}

	// Default search for "pounds weight" excludes the superseded old note but
	// surfaces the current correction and the independent time-bound fact.
	hits, err := store.SearchNotes(ctx, tenantID, "weighs pounds weight", index.SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if hasHit(hits, "syn:weight") {
		t.Errorf("superseded note syn:weight leaked into default retrieval: %v", ids(hits))
	}
	if !hasHit(hits, "syn:weight-2026") {
		t.Errorf("current note syn:weight-2026 missing from retrieval: %v", ids(hits))
	}
	if !hasHit(hits, "syn:weight-2022") {
		t.Errorf("coexisting time-bound note syn:weight-2022 missing: %v", ids(hits))
	}

	// With IncludeSuperseded the stale note is retained and reachable (history).
	histHits, err := store.SearchNotes(ctx, tenantID, "weighs pounds weight",
		index.SearchOpts{Limit: 10, IncludeSuperseded: true})
	if err != nil {
		t.Fatalf("history search: %v", err)
	}
	if !hasHit(histHits, "syn:weight") {
		t.Errorf("superseded note not reachable with IncludeSuperseded: %v", ids(histHits))
	}

	// Self-heal: delete the superseding note; the target becomes current again.
	if _, err := store.DeleteNote(ctx, tenantID, "syn:weight-2026", 1, "test"); err != nil {
		t.Fatalf("delete superseding note: %v", err)
	}
	if _, err := store.ReconcileSupersessions(ctx, tenantID); err != nil {
		t.Fatalf("reconcile after delete: %v", err)
	}
	hits, err = store.SearchNotes(ctx, tenantID, "weighs pounds weight", index.SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if !hasHit(hits, "syn:weight") {
		t.Errorf("target not resurrected after superseding note removed: %v", ids(hits))
	}
}

// TestSupersedeRetargetResurrectsOldTarget verifies the scoped per-write reconcile
// recomputes the OLD supersedes target when a note's pointer is moved, not just
// the new one — so re-pointing a correction off note A onto note B un-hides A and
// hides B, all from the single write's in-tx reconcile.
func TestSupersedeRetargetResurrectsOldTarget(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	seedMarkdown(t, store, tenantID, "a.md", "---\nid: syn:a\ntitle: A\n---\n# A\n\nalpha fact body.\n", false)
	seedMarkdown(t, store, tenantID, "b.md", "---\nid: syn:b\ntitle: B\n---\n# B\n\nbravo fact body.\n", false)
	seedMarkdown(t, store, tenantID, "c.md", "---\nid: syn:c\ntitle: C\nsupersedes: syn:a\n---\n# C\n\ncharlie correction.\n", false)

	if !supersededFlag(t, store, tenantID, "syn:a") {
		t.Fatal("A should be superseded by C after seeding")
	}

	// Re-point C from A to B (an update at version 1, supersedes now syn:b).
	if _, err := store.WriteNote(ctx, tenantID, &index.PGNoteInput{
		ID: "syn:c", Title: "C", Body: "charlie correction v2.", Supersedes: "syn:b", Surface: "test",
	}, 1); err != nil {
		t.Fatalf("retarget write: %v", err)
	}
	if supersededFlag(t, store, tenantID, "syn:a") {
		t.Error("A should be resurrected after C re-pointed away from it")
	}
	if !supersededFlag(t, store, tenantID, "syn:b") {
		t.Error("B should be superseded after C re-pointed onto it")
	}
}

// TestWriteSupersedesMissingIsAtomic verifies a write whose supersedes points at
// a non-existent note is rejected as *SupersedesMissingError and writes nothing
// (the check runs inside the write transaction).
func TestWriteSupersedesMissingIsAtomic(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	_, err := store.WriteNote(ctx, tenantID, &index.PGNoteInput{
		ID: "syn:orphan", Title: "Orphan", Body: "points at nothing.", Supersedes: "syn:ghost", Surface: "test",
	}, 0)
	var missing *index.SupersedesMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("want *SupersedesMissingError, got %v", err)
	}
	if _, found, err := store.NoteState(ctx, tenantID, "syn:orphan"); err != nil {
		t.Fatalf("note state: %v", err)
	} else if found {
		t.Error("note was written despite the missing supersedes target — not atomic")
	}
}

// supersededFlag returns a note's current superseded flag via a list that
// includes superseded notes.
func supersededFlag(t *testing.T, store *index.Store, tenantID, id string) bool {
	t.Helper()
	sums, err := store.ListNotes(context.Background(), tenantID, index.ListOpts{IncludeSuperseded: true, Limit: 500})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	for i := range sums {
		if sums[i].ID == id {
			return sums[i].Superseded
		}
	}
	t.Fatalf("note %q not found in list", id)
	return false
}

func hasHit(hits []index.NoteHit, id string) bool {
	for i := range hits {
		if hits[i].ID == id {
			return true
		}
	}
	return false
}

func ids(hits []index.NoteHit) []string {
	out := make([]string, len(hits))
	for i := range hits {
		out[i] = hits[i].ID
	}
	return out
}
