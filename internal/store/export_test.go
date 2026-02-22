package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestExportAllNodes_Empty(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	nodes, err := es.ExportAllNodes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllNodes: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for fresh tenant, got %d", len(nodes))
	}
}

func TestExportAllNodes_ReturnsDecryptedProperties(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	node := models.ExportNode{
		ID:            "node-export-1",
		Type:          "person",
		Label:         "Alice",
		Properties:    map[string]any{"age": float64(30), "role": "admin"},
		SalienceScore: 0.8,
		UserBoosted:   false,
		CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
	}

	action, err := es.UpsertNodeFromExport(ctx, tenantID, node, false)
	if err != nil {
		t.Fatalf("UpsertNodeFromExport: %v", err)
	}

	if action != "created" {
		t.Errorf("expected action 'created', got %q", action)
	}

	got, err := es.ExportAllNodes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllNodes: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 node, got %d", len(got))
	}

	n := got[0]

	if n.ID != "node-export-1" {
		t.Errorf("ID = %q, want %q", n.ID, "node-export-1")
	}

	if n.Type != "person" {
		t.Errorf("Type = %q, want %q", n.Type, "person")
	}

	if n.Properties["age"] != float64(30) {
		t.Errorf("Properties[age] = %v, want 30", n.Properties["age"])
	}

	if n.Properties["role"] != "admin" {
		t.Errorf("Properties[role] = %v, want 'admin'", n.Properties["role"])
	}
}

func TestExportAllNodes_SortedByCreatedAt(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	now := time.Now().UTC()

	nodes := []models.ExportNode{
		{ID: "b", Type: "t", Label: "B", Properties: map[string]any{}, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
		{ID: "a", Type: "t", Label: "A", Properties: map[string]any{}, CreatedAt: now, UpdatedAt: now},
	}

	for _, n := range nodes {
		if _, err := es.UpsertNodeFromExport(ctx, tenantID, n, false); err != nil {
			t.Fatalf("UpsertNodeFromExport(%s): %v", n.ID, err)
		}
	}

	got, err := es.ExportAllNodes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllNodes: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got))
	}

	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("unexpected order: got [%s, %s], want [a, b]", got[0].ID, got[1].ID)
	}
}

func TestExportAllEdges_Empty(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	edges, err := es.ExportAllEdges(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllEdges: %v", err)
	}

	if len(edges) != 0 {
		t.Errorf("expected 0 edges for fresh tenant, got %d", len(edges))
	}
}

func TestExportAllEdges_ReturnsDecryptedProperties(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	edge := models.ExportEdge{
		Source:     "src",
		Target:     "tgt",
		Relation:   "knows",
		Properties: map[string]any{"since": "2024", "strength": float64(0.9)},
		Weight:     0.75,
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := es.UpsertEdgeFromExport(ctx, tenantID, edge, false); err != nil {
		t.Fatalf("UpsertEdgeFromExport: %v", err)
	}

	got, err := es.ExportAllEdges(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllEdges: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(got))
	}

	e := got[0]

	if e.Source != "src" || e.Target != "tgt" || e.Relation != "knows" {
		t.Errorf("unexpected edge key: %s→%s (%s)", e.Source, e.Target, e.Relation)
	}

	if e.Properties["since"] != "2024" {
		t.Errorf("Properties[since] = %v, want '2024'", e.Properties["since"])
	}

	if e.Properties["strength"] != float64(0.9) {
		t.Errorf("Properties[strength] = %v, want 0.9", e.Properties["strength"])
	}
}

func TestExportAllEdges_SortedBySourceTargetRelation(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	edges := []models.ExportEdge{
		{Source: "b", Target: "c", Relation: "r", Properties: map[string]any{}, Weight: 1.0, CreatedAt: now, UpdatedAt: now},
		{Source: "a", Target: "c", Relation: "r", Properties: map[string]any{}, Weight: 1.0, CreatedAt: now, UpdatedAt: now},
		{Source: "a", Target: "b", Relation: "r", Properties: map[string]any{}, Weight: 1.0, CreatedAt: now, UpdatedAt: now},
	}

	for _, e := range edges {
		if _, err := es.UpsertEdgeFromExport(ctx, tenantID, e, false); err != nil {
			t.Fatalf("UpsertEdgeFromExport(%s→%s): %v", e.Source, e.Target, err)
		}
	}

	got, err := es.ExportAllEdges(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllEdges: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(got))
	}

	// Expect ORDER BY source, target, relation: a→b, a→c, b→c.
	expected := [][2]string{{"a", "b"}, {"a", "c"}, {"b", "c"}}
	for i, pair := range expected {
		if got[i].Source != pair[0] || got[i].Target != pair[1] {
			t.Errorf("edge[%d] = %s→%s, want %s→%s", i, got[i].Source, got[i].Target, pair[0], pair[1])
		}
	}
}
