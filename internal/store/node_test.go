package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestCreateNode(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{
		Type:       "person",
		Label:      "Alice Test",
		Properties: map[string]any{"age": float64(30)},
	}
	_ = req.Validate()

	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	if node.Type != "person" {
		t.Errorf("Type = %q, want %q", node.Type, "person")
	}
	if node.Label != "Alice Test" {
		t.Errorf("Label = %q, want %q", node.Label, "Alice Test")
	}
	if node.ID == "" {
		t.Error("ID is empty")
	}
	if node.Properties["age"] != float64(30) {
		t.Errorf("Properties[age] = %v, want 30", node.Properties["age"])
	}
}

func TestGetNode(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "concept", Label: "Roundtrip Test"}
	_ = req.Validate()

	created, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	got, err := ns.GetNode(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Label != "Roundtrip Test" {
		t.Errorf("Label = %q, want %q", got.Label, "Roundtrip Test")
	}
	if got.Type != "concept" {
		t.Errorf("Type = %q, want %q", got.Type, "concept")
	}
}

func TestUpdateNode(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "concept", Label: "Before Update"}
	_ = req.Validate()

	created, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	newLabel := "After Update"
	newType := "entity"
	updated, err := ns.UpdateNode(ctx, tenantID, created.ID, models.UpdateNodeRequest{
		Label:      &newLabel,
		Type:       &newType,
		Properties: map[string]any{"updated": true},
	})
	if err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	if updated.Label != "After Update" {
		t.Errorf("Label = %q, want %q", updated.Label, "After Update")
	}
	if updated.Type != "entity" {
		t.Errorf("Type = %q, want %q", updated.Type, "entity")
	}
	if updated.Properties["updated"] != true {
		t.Errorf("Properties[updated] = %v, want true", updated.Properties["updated"])
	}
}

func TestDeleteNode(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "concept", Label: "To Delete"}
	_ = req.Validate()

	created, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	if err := ns.DeleteNode(ctx, tenantID, created.ID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	_, err = ns.GetNode(ctx, tenantID, created.ID)
	if !errors.Is(err, models.ErrNodeNotFound) {
		t.Errorf("GetNode after delete: got %v, want ErrNodeNotFound", err)
	}
}

func TestListNodes(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	for _, label := range []string{"List A", "List B", "List C"} {
		req := models.CreateNodeRequest{Type: "concept", Label: label}
		_ = req.Validate()
		if _, err := ns.CreateNode(ctx, tenantID, req); err != nil {
			t.Fatalf("CreateNode(%s): %v", label, err)
		}
	}

	nodes, hasMore, err := ns.ListNodes(ctx, tenantID, "", 0, 50, 0)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	if len(nodes) != 3 {
		t.Errorf("ListNodes returned %d nodes, want 3", len(nodes))
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}

	// Filter by type.
	filtered, _, err := ns.ListNodes(ctx, tenantID, "nonexistent", 0, 50, 0)
	if err != nil {
		t.Fatalf("ListNodes with filter: %v", err)
	}
	if len(filtered) != 0 {
		t.Errorf("filtered ListNodes returned %d, want 0", len(filtered))
	}
}

func TestEncryptionRoundtrip(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	props := map[string]any{
		"secret":  "classified-data",
		"nested":  map[string]any{"deep": "value"},
		"numbers": float64(42),
	}

	req := models.CreateNodeRequest{Type: "secret", Label: "Encrypted Node", Properties: props}
	_ = req.Validate()

	created, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	got, err := ns.GetNode(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if got.Properties["secret"] != "classified-data" {
		t.Errorf("Properties[secret] = %v, want classified-data", got.Properties["secret"])
	}
	if got.Properties["numbers"] != float64(42) {
		t.Errorf("Properties[numbers] = %v, want 42", got.Properties["numbers"])
	}

	nested, ok := got.Properties["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Properties[nested] is not map, got %T", got.Properties["nested"])
	}
	if nested["deep"] != "value" {
		t.Errorf("Properties[nested][deep] = %v, want value", nested["deep"])
	}
}
