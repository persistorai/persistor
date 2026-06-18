package index_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestReconcileSupersessions checks supersession: a superseding note
// hides the stale note from default retrieval, the stale note is still
// retrievable with IncludeSuperseded, and an unrelated time-bound note coexists
// (is never superseded). It also checks self-healing: deleting the superseding
// note's file resurrects the target.
func TestReconcileSupersessions(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	// Old fact, the current correction that supersedes it, and an independent
	// time-bound fact that must NOT be superseded (both true at their time).
	writeFile(t, dir, "weight-old.md",
		"---\nid: syn:weight\ntitle: Weight\n---\n# Weight\n\nWeighs 250 pounds at the desk.\n")
	writeFile(t, dir, "weight-new.md",
		"---\nid: syn:weight-2026\ntitle: Weight 2026\nsupersedes: syn:weight\n---\n# Weight 2026\n\nWeighs 230 pounds now.\n")
	writeFile(t, dir, "weight-2022.md",
		"---\nid: syn:weight-2022\ntitle: Weight 2022\n---\n# Weight 2022\n\nWeighed 190 pounds in 2022 in Oklahoma.\n")
	roots := []index.Root{{Name: "syn", Dir: dir}}

	rep, err := ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if rep.Notes != 3 {
		t.Fatalf("want 3 notes, got %d", rep.Notes)
	}
	if rep.Superseded != 1 {
		t.Errorf("want 1 supersession change, got %d", rep.Superseded)
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

	// Self-heal: drop the superseding file; the target becomes current again.
	rmFile(t, dir, "weight-new.md")
	rep, err = ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("reindex after delete: %v", err)
	}
	if rep.Deleted != 1 {
		t.Errorf("want 1 deleted, got %d", rep.Deleted)
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
