package store_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestFullTextSearch(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ss := store.NewSearchStore(base)
	ctx := context.Background()

	// Create nodes with distinctive labels for full-text search.
	for _, label := range []string{
		"Quantum photosynthesis research",
		"Quantum entanglement experiment",
		"Classical music composition",
	} {
		req := models.CreateNodeRequest{Type: "concept", Label: label}
		_ = req.Validate()
		if _, err := ns.CreateNode(ctx, tenantID, req); err != nil {
			t.Fatalf("CreateNode(%s): %v", label, err)
		}
	}

	// Search for "quantum" — should find 2 nodes.
	results, err := ss.FullTextSearch(ctx, tenantID, "quantum", "", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("FullTextSearch(quantum) = %d results, want 2", len(results))
	}

	// Search for "classical" — should find 1 node.
	results, err = ss.FullTextSearch(ctx, tenantID, "classical", "", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("FullTextSearch(classical) = %d results, want 1", len(results))
	}

	// Search with type filter.
	results, err = ss.FullTextSearch(ctx, tenantID, "quantum", "nonexistent", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch with type filter: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("FullTextSearch with bad type filter = %d results, want 0", len(results))
	}
}
