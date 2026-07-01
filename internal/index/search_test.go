package index_test

import (
	"context"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestSearchNotes_FindsTailFactAndRanks(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	seedMarkdown(t, store, tenantID, "aurora.md", "# Aurora Protocol\n\nSafety protocol for navigating polar storms.\n", false)
	seedMarkdown(t, store, tenantID, "helios.md", "# Helios Engine\n\nSolar propulsion for cargo airships.\n", false)

	hits, err := store.SearchNotes(ctx, tenantID, "polar storm navigation protocol", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "syn:aurora" {
		t.Fatalf("expected syn:aurora ranked first, got %+v", hits)
	}
}

func TestSearchNotes_TierFilter(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	seedMarkdown(t, store, tenantID, "core.md", "# Core\n\nThe mission is to chart trade routes.\n", true)
	seedMarkdown(t, store, tenantID, "tail.md", "# Tail\n\nThe mission detail lives here in a tail note.\n", false)

	all, err := store.SearchNotes(ctx, tenantID, "mission", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered search = %d notes, want 2", len(all))
	}

	coreOnly, err := store.SearchNotes(ctx, tenantID, "mission", &index.SearchOpts{Limit: 5, Tier: "core"})
	if err != nil {
		t.Fatalf("search core: %v", err)
	}
	if len(coreOnly) != 1 || coreOnly[0].ID != "syn:core" {
		t.Errorf("core-only search = %+v, want just syn:core", coreOnly)
	}
}

func TestSearchNotes_EmptyQueryMatchesNothing(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()
	seedMarkdown(t, store, tenantID, "a.md", "# A\n\nContent.\n", false)
	hits, err := store.SearchNotes(ctx, tenantID, "  ... !! ", &index.SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("punctuation-only query should match nothing, got %d", len(hits))
	}
}
