package store_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestNeighbors(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	center := createTestNode(t, ns, tenantID, "Center Node")
	n1 := createTestNode(t, ns, tenantID, "Neighbor 1")
	n2 := createTestNode(t, ns, tenantID, "Neighbor 2")

	for _, e := range []models.CreateEdgeRequest{
		{Source: center.ID, Target: n1.ID, Relation: "connects"},
		{Source: n2.ID, Target: center.ID, Relation: "connects"},
	} {
		if _, err := es.CreateEdge(ctx, tenantID, e); err != nil {
			t.Fatalf("CreateEdge: %v", err)
		}
	}

	result, err := gs.Neighbors(ctx, tenantID, center.ID, 100)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Errorf("Neighbors nodes = %d, want 2", len(result.Nodes))
	}
	if len(result.Edges) != 2 {
		t.Errorf("Neighbors edges = %d, want 2", len(result.Edges))
	}
}

func TestTraverse(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	// Build A → B → C chain.
	a := createTestNode(t, ns, tenantID, "Traverse A")
	b := createTestNode(t, ns, tenantID, "Traverse B")
	c := createTestNode(t, ns, tenantID, "Traverse C")

	if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source: a.ID, Target: b.ID, Relation: "next",
	}); err != nil {
		t.Fatalf("CreateEdge A→B: %v", err)
	}
	if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source: b.ID, Target: c.ID, Relation: "next",
	}); err != nil {
		t.Fatalf("CreateEdge B→C: %v", err)
	}

	// Depth 1 from A should find A and B.
	r1, err := gs.Traverse(ctx, tenantID, a.ID, 1)
	if err != nil {
		t.Fatalf("Traverse depth 1: %v", err)
	}
	if len(r1.Nodes) != 2 {
		t.Errorf("Traverse depth 1 nodes = %d, want 2", len(r1.Nodes))
	}

	// Depth 2 from A should find A, B, and C.
	r2, err := gs.Traverse(ctx, tenantID, a.ID, 2)
	if err != nil {
		t.Fatalf("Traverse depth 2: %v", err)
	}
	if len(r2.Nodes) != 3 {
		t.Errorf("Traverse depth 2 nodes = %d, want 3", len(r2.Nodes))
	}
	if len(r2.Edges) != 2 {
		t.Errorf("Traverse depth 2 edges = %d, want 2", len(r2.Edges))
	}
}

func TestGraphContext(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	center := createTestNode(t, ns, tenantID, "Context Center")
	friend := createTestNode(t, ns, tenantID, "Context Friend")

	if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source: center.ID, Target: friend.ID, Relation: "knows",
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	result, err := gs.GraphContext(ctx, tenantID, center.ID)
	if err != nil {
		t.Fatalf("GraphContext: %v", err)
	}

	if result.Node.ID != center.ID {
		t.Errorf("GraphContext node = %q, want %q", result.Node.ID, center.ID)
	}
	if len(result.Neighbors) != 1 {
		t.Errorf("GraphContext neighbors = %d, want 1", len(result.Neighbors))
	}
	if len(result.Edges) != 1 {
		t.Errorf("GraphContext edges = %d, want 1", len(result.Edges))
	}
}
