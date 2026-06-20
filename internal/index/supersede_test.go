package index_test

import (
	"context"
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

	changed, err := store.ReconcileSupersessions(ctx, tenantID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if changed != 1 {
		t.Errorf("want 1 supersession change, got %d", changed)
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
