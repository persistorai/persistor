package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestUpsertNodeFromExport_Creates(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	node := models.ExportNode{
		ID:         "import-node-1",
		Type:       "person",
		Label:      "Bob",
		Properties: map[string]any{"email": "bob@example.com"},
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	action, err := es.UpsertNodeFromExport(ctx, tenantID, node, false)
	if err != nil {
		t.Fatalf("UpsertNodeFromExport: %v", err)
	}

	if action != "created" {
		t.Errorf("action = %q, want 'created'", action)
	}
}

func TestUpsertNodeFromExport_SkipsExisting(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	node := models.ExportNode{
		ID:        "import-node-skip",
		Type:      "concept",
		Label:     "Original",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := es.UpsertNodeFromExport(ctx, tenantID, node, false); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	action, err := es.UpsertNodeFromExport(ctx, tenantID, node, false)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if action != "skipped" {
		t.Errorf("action = %q, want 'skipped'", action)
	}
}

func TestUpsertNodeFromExport_OverwritesExisting(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	node := models.ExportNode{
		ID: "import-node-overwrite", Type: "concept", Label: "Original",
		Properties: map[string]any{"v": "before"}, CreatedAt: now, UpdatedAt: now,
	}

	if _, err := es.UpsertNodeFromExport(ctx, tenantID, node, false); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	updated := node
	updated.Label = "Replaced"
	updated.Properties = map[string]any{"v": "after"}

	action, err := es.UpsertNodeFromExport(ctx, tenantID, updated, true)
	if err != nil {
		t.Fatalf("overwrite upsert: %v", err)
	}

	if action != "updated" {
		t.Errorf("action = %q, want 'updated'", action)
	}

	// Verify the updated data is readable via export.
	nodes, err := es.ExportAllNodes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllNodes: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	if nodes[0].Label != "Replaced" {
		t.Errorf("Label = %q, want 'Replaced'", nodes[0].Label)
	}

	if nodes[0].Properties["v"] != "after" {
		t.Errorf("Properties[v] = %v, want 'after'", nodes[0].Properties["v"])
	}
}

func TestUpsertEdgeFromExport_Creates(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	edge := models.ExportEdge{
		Source: "x", Target: "y", Relation: "linked",
		Properties: map[string]any{"w": "val"},
		Weight:     0.5,
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	action, err := es.UpsertEdgeFromExport(ctx, tenantID, edge, false)
	if err != nil {
		t.Fatalf("UpsertEdgeFromExport: %v", err)
	}

	if action != "created" {
		t.Errorf("action = %q, want 'created'", action)
	}
}

func TestUpsertEdgeFromExport_SkipsExisting(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	edge := models.ExportEdge{
		Source: "p", Target: "q", Relation: "rel",
		Properties: map[string]any{}, Weight: 1.0,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := es.UpsertEdgeFromExport(ctx, tenantID, edge, false); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	action, err := es.UpsertEdgeFromExport(ctx, tenantID, edge, false)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if action != "skipped" {
		t.Errorf("action = %q, want 'skipped'", action)
	}
}

func TestUpsertEdgeFromExport_OverwritesExisting(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	edge := models.ExportEdge{
		Source: "m", Target: "n", Relation: "r",
		Properties: map[string]any{}, Weight: 1.0,
		CreatedAt: now, UpdatedAt: now,
	}

	if _, err := es.UpsertEdgeFromExport(ctx, tenantID, edge, false); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	updated := edge
	updated.Weight = 0.25
	updated.Properties = map[string]any{"updated": true}

	action, err := es.UpsertEdgeFromExport(ctx, tenantID, updated, true)
	if err != nil {
		t.Fatalf("overwrite upsert: %v", err)
	}

	if action != "updated" {
		t.Errorf("action = %q, want 'updated'", action)
	}

	// Verify the updated weight is readable via export.
	edges, err := es.ExportAllEdges(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllEdges: %v", err)
	}

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	if edges[0].Weight != 0.25 {
		t.Errorf("Weight = %v, want 0.25", edges[0].Weight)
	}
}

func TestUpsertNodeFromExport_EncryptionRoundtrip(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewExportStore(base)
	ctx := context.Background()

	props := map[string]any{
		"secret":  "classified",
		"numbers": float64(42),
		"nested":  map[string]any{"deep": "value"},
	}

	node := models.ExportNode{
		ID:         "enc-roundtrip",
		Type:       "secret",
		Label:      "Encrypted",
		Properties: props,
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := es.UpsertNodeFromExport(ctx, tenantID, node, false); err != nil {
		t.Fatalf("UpsertNodeFromExport: %v", err)
	}

	nodes, err := es.ExportAllNodes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ExportAllNodes: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	if nodes[0].Properties["secret"] != "classified" {
		t.Errorf("Properties[secret] = %v, want 'classified'", nodes[0].Properties["secret"])
	}

	if nodes[0].Properties["numbers"] != float64(42) {
		t.Errorf("Properties[numbers] = %v, want 42", nodes[0].Properties["numbers"])
	}

	nested, ok := nodes[0].Properties["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Properties[nested] not a map, got %T", nodes[0].Properties["nested"])
	}

	if nested["deep"] != "value" {
		t.Errorf("Properties[nested][deep] = %v, want 'value'", nested["deep"])
	}
}
