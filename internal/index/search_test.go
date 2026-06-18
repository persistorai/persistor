package index_test

import (
	"context"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestSearchNotes_FindsTailFactAndRanks(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, dir, "aurora.md", "# Aurora Protocol\n\nSafety protocol for navigating polar storms.\n")
	writeFile(t, dir, "helios.md", "# Helios Engine\n\nSolar propulsion for cargo airships.\n")
	if _, err := ix.Reindex(ctx, tenantID, []index.Root{{Name: "syn", Dir: dir}}); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	hits, err := store.SearchNotes(ctx, tenantID, "polar storm navigation protocol", index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "syn:aurora" {
		t.Fatalf("expected syn:aurora ranked first, got %+v", hits)
	}
}

func TestSearchNotes_TierFilter(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, dir, "core.md", "# Core\n\nThe mission is to chart trade routes.\n")
	writeFile(t, dir, "tail.md", "# Tail\n\nThe mission detail lives here in a tail note.\n")
	roots := []index.Root{{Name: "syn", Dir: dir, CorePaths: []string{"core.md"}}}
	if _, err := ix.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	all, err := store.SearchNotes(ctx, tenantID, "mission", index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered search = %d notes, want 2", len(all))
	}

	coreOnly, err := store.SearchNotes(ctx, tenantID, "mission", index.SearchOpts{Limit: 5, Tier: "core"})
	if err != nil {
		t.Fatalf("search core: %v", err)
	}
	if len(coreOnly) != 1 || coreOnly[0].ID != "syn:core" {
		t.Errorf("core-only search = %+v, want just syn:core", coreOnly)
	}
}

func TestSearchNotes_EmptyQueryMatchesNothing(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# A\n\nContent.\n")
	if _, err := ix.Reindex(ctx, tenantID, []index.Root{{Name: "syn", Dir: dir}}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	hits, err := store.SearchNotes(ctx, tenantID, "  ... !! ", index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("punctuation-only query should match nothing, got %d", len(hits))
	}
}
