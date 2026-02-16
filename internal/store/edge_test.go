package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func createTestNode(t *testing.T, ns *store.NodeStore, tenantID, label string) *models.Node {
	t.Helper()
	req := models.CreateNodeRequest{Type: "concept", Label: label}
	_ = req.Validate()

	n, err := ns.CreateNode(context.Background(), tenantID, req)
	if err != nil {
		t.Fatalf("createTestNode(%s): %v", label, err)
	}

	return n
}

func TestCreateEdge(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	ctx := context.Background()

	src := createTestNode(t, ns, tenantID, "Edge Source")
	tgt := createTestNode(t, ns, tenantID, "Edge Target")

	edge, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source:   src.ID,
		Target:   tgt.ID,
		Relation: "related_to",
	})
	if err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if edge.Source != src.ID {
		t.Errorf("Source = %q, want %q", edge.Source, src.ID)
	}
	if edge.Target != tgt.ID {
		t.Errorf("Target = %q, want %q", edge.Target, tgt.ID)
	}
	if edge.Relation != "related_to" {
		t.Errorf("Relation = %q, want %q", edge.Relation, "related_to")
	}
	if edge.Weight != 1.0 {
		t.Errorf("Weight = %f, want 1.0", edge.Weight)
	}
}

func TestDeleteEdge(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	ctx := context.Background()

	src := createTestNode(t, ns, tenantID, "Del Edge Src")
	tgt := createTestNode(t, ns, tenantID, "Del Edge Tgt")

	_, err := es.CreateEdge(ctx, tenantID, models.CreateEdgeRequest{
		Source: src.ID, Target: tgt.ID, Relation: "test_rel",
	})
	if err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if err := es.DeleteEdge(ctx, tenantID, src.ID, tgt.ID, "test_rel"); err != nil {
		t.Fatalf("DeleteEdge: %v", err)
	}

	// Verify gone.
	err = es.DeleteEdge(ctx, tenantID, src.ID, tgt.ID, "test_rel")
	if !errors.Is(err, models.ErrEdgeNotFound) {
		t.Errorf("second DeleteEdge: got %v, want ErrEdgeNotFound", err)
	}
}

func TestListEdges(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	es := store.NewEdgeStore(base)
	ctx := context.Background()

	a := createTestNode(t, ns, tenantID, "LE Node A")
	b := createTestNode(t, ns, tenantID, "LE Node B")
	c := createTestNode(t, ns, tenantID, "LE Node C")

	for _, e := range []models.CreateEdgeRequest{
		{Source: a.ID, Target: b.ID, Relation: "knows"},
		{Source: a.ID, Target: c.ID, Relation: "knows"},
		{Source: b.ID, Target: c.ID, Relation: "likes"},
	} {
		if _, err := es.CreateEdge(ctx, tenantID, e); err != nil {
			t.Fatalf("CreateEdge: %v", err)
		}
	}

	// All edges.
	all, _, err := es.ListEdges(ctx, tenantID, "", "", "", 50, 0)
	if err != nil {
		t.Fatalf("ListEdges all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListEdges all = %d, want 3", len(all))
	}

	// Filter by source.
	bySource, _, err := es.ListEdges(ctx, tenantID, a.ID, "", "", 50, 0)
	if err != nil {
		t.Fatalf("ListEdges by source: %v", err)
	}
	if len(bySource) != 2 {
		t.Errorf("ListEdges by source = %d, want 2", len(bySource))
	}

	// Filter by relation.
	byRel, _, err := es.ListEdges(ctx, tenantID, "", "", "likes", 50, 0)
	if err != nil {
		t.Fatalf("ListEdges by relation: %v", err)
	}
	if len(byRel) != 1 {
		t.Errorf("ListEdges by relation = %d, want 1", len(byRel))
	}
}
