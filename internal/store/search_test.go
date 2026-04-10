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

	// Search should also hit indexed property text, not just labels.
	propertyReq := models.CreateNodeRequest{
		Type:  "project",
		Label: "Persistor",
		Properties: map[string]any{
			"summary": "Memory system for AI agents",
		},
	}
	_ = propertyReq.Validate()
	if _, err := ns.CreateNode(ctx, tenantID, propertyReq); err != nil {
		t.Fatalf("CreateNode(propertyReq): %v", err)
	}

	results, err = ss.FullTextSearch(ctx, tenantID, "agents", "", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch(property text): %v", err)
	}

	if len(results) != 1 || results[0].Label != "Persistor" {
		t.Fatalf("FullTextSearch(property text) = %#v, want Persistor hit", results)
	}
}

func TestFullTextSearch_UsesAliasCandidates(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	as := store.NewAliasStore(base)
	ss := store.NewSearchStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "person", Label: "William Gates"}
	_ = req.Validate()
	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	if _, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{NodeID: node.ID, Alias: "Bill Gates", AliasType: "nickname"}); err != nil {
		t.Fatalf("CreateAlias nickname: %v", err)
	}
	if _, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{NodeID: node.ID, Alias: "  William H. Gates  ", AliasType: "full_name"}); err != nil {
		t.Fatalf("CreateAlias full_name: %v", err)
	}

	results, err := ss.FullTextSearch(ctx, tenantID, "Bill Gates", "", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch exact alias: %v", err)
	}
	if len(results) != 1 || results[0].ID != node.ID {
		t.Fatalf("FullTextSearch exact alias = %#v, want node %q", results, node.ID)
	}

	results, err = ss.FullTextSearch(ctx, tenantID, "william h. gates", "", 0, 10)
	if err != nil {
		t.Fatalf("FullTextSearch normalized alias: %v", err)
	}
	if len(results) != 1 || results[0].ID != node.ID {
		t.Fatalf("FullTextSearch normalized alias = %#v, want node %q", results, node.ID)
	}
}

func TestHybridSearch_UsesAliasCandidates(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	as := store.NewAliasStore(base)
	ss := store.NewSearchStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "person", Label: "Samuel Clemens"}
	_ = req.Validate()
	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	if _, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{NodeID: node.ID, Alias: "Mark Twain", AliasType: "pen_name"}); err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	results, err := ss.HybridSearch(ctx, tenantID, "Mark Twain", []float32{0.1, 0.2}, 10)
	if err != nil {
		t.Fatalf("HybridSearch alias: %v", err)
	}
	if len(results) != 1 || results[0].ID != node.ID {
		t.Fatalf("HybridSearch alias = %#v, want node %q", results, node.ID)
	}
}
