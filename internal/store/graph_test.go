package store_test

import (
	"context"
	"errors"
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

func TestTraverseNodeLimitKeepsEdgesConsistent(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	root := createTestNode(t, ns, tenantID, "Traverse limit root")
	for i := range 600 {
		neighbor := createTestNode(t, ns, tenantID, "Traverse limit neighbor")
		if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
			Source:   root.ID,
			Target:   neighbor.ID,
			Relation: "fanout",
		}); err != nil {
			t.Fatalf("CreateEdge %d: %v", i, err)
		}
	}

	result, err := gs.Traverse(ctx, tenantID, root.ID, 1)
	if err != nil {
		t.Fatalf("Traverse depth 1: %v", err)
	}
	if len(result.Nodes) != 500 {
		t.Fatalf("Traverse nodes = %d, want 500", len(result.Nodes))
	}

	nodeIDs := make(map[string]struct{}, len(result.Nodes))
	for _, node := range result.Nodes {
		nodeIDs[node.ID] = struct{}{}
	}

	for _, edge := range result.Edges {
		if _, ok := nodeIDs[edge.Source]; !ok {
			t.Fatalf("edge source %q missing from node set", edge.Source)
		}
		if _, ok := nodeIDs[edge.Target]; !ok {
			t.Fatalf("edge target %q missing from node set", edge.Target)
		}
	}
}

func TestShortestPathMissingNodeReturnsErrNodeNotFound(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	existing := createTestNode(t, ns, tenantID, "Path existing")

	if _, err := gs.ShortestPath(ctx, tenantID, existing.ID, "missing-node"); !errors.Is(err, models.ErrNodeNotFound) {
		t.Fatalf("ShortestPath missing target err = %v, want ErrNodeNotFound", err)
	}

	if _, err := gs.ShortestPath(ctx, tenantID, "missing-node", existing.ID); !errors.Is(err, models.ErrNodeNotFound) {
		t.Fatalf("ShortestPath missing source err = %v, want ErrNodeNotFound", err)
	}
}

func TestShortestPathDoesNotDropLargeFrontierBranches(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	gs := store.NewGraphStore(base)
	ctx := context.Background()

	start := createTestNode(t, ns, tenantID, "Path start")
	target := createTestNode(t, ns, tenantID, "Path target")

	var branchWithTarget string
	for i := range 600 {
		mid := createTestNode(t, ns, tenantID, "Path mid")
		if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
			Source:   start.ID,
			Target:   mid.ID,
			Relation: "branch",
		}); err != nil {
			t.Fatalf("CreateEdge start->mid %d: %v", i, err)
		}
		if i == 599 {
			branchWithTarget = mid.ID
		}
	}

	if _, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source:   branchWithTarget,
		Target:   target.ID,
		Relation: "branch",
	}); err != nil {
		t.Fatalf("CreateEdge mid->target: %v", err)
	}

	path, err := gs.ShortestPath(ctx, tenantID, start.ID, target.ID)
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
	if len(path) != 3 {
		t.Fatalf("ShortestPath length = %d, want 3", len(path))
	}
	if path[0].ID != start.ID || path[1].ID != branchWithTarget || path[2].ID != target.ID {
		t.Fatalf("ShortestPath = [%q %q %q], want [%q %q %q]", path[0].ID, path[1].ID, path[2].ID, start.ID, branchWithTarget, target.ID)
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
