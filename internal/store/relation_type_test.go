package store_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/store"
)

func TestRelationTypeStore_ListRelationTypes(t *testing.T) {
	base, tenantID := setupTestBase(t)
	rts := store.NewRelationTypeStore(base)
	ctx := context.Background()

	types, err := rts.ListRelationTypes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRelationTypes: %v", err)
	}

	// Should include at least the 37 canonical types.
	if len(types) < 37 {
		t.Errorf("ListRelationTypes returned %d types, want >= 37", len(types))
	}
}

func TestRelationTypeStore_AddRelationType(t *testing.T) {
	base, tenantID := setupTestBase(t)
	rts := store.NewRelationTypeStore(base)
	ctx := context.Background()

	rt, err := rts.AddRelationType(ctx, tenantID, "custom_test_rel", "A test relation")
	if err != nil {
		t.Fatalf("AddRelationType: %v", err)
	}

	if rt.Name != "custom_test_rel" {
		t.Errorf("Name = %q, want %q", rt.Name, "custom_test_rel")
	}

	if rt.Description != "A test relation" {
		t.Errorf("Description = %q, want %q", rt.Description, "A test relation")
	}

	// Verify it shows up in list.
	types, err := rts.ListRelationTypes(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRelationTypes after add: %v", err)
	}

	found := false
	for _, typ := range types {
		if typ.Name == "custom_test_rel" {
			found = true

			break
		}
	}

	if !found {
		t.Error("custom_test_rel not found in ListRelationTypes")
	}
}

func TestRelationTypeStore_AddRelationType_EmptyName(t *testing.T) {
	base, tenantID := setupTestBase(t)
	rts := store.NewRelationTypeStore(base)
	ctx := context.Background()

	_, err := rts.AddRelationType(ctx, tenantID, "", "some desc")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRelationTypeStore_IsCanonical(t *testing.T) {
	base, _ := setupTestBase(t)
	rts := store.NewRelationTypeStore(base)
	ctx := context.Background()

	ok, err := rts.IsCanonical(ctx, "created")
	if err != nil {
		t.Fatalf("IsCanonical(created): %v", err)
	}

	if !ok {
		t.Error("expected 'created' to be canonical")
	}

	ok, err = rts.IsCanonical(ctx, "nonexistent_relation_xyz")
	if err != nil {
		t.Fatalf("IsCanonical(nonexistent): %v", err)
	}

	if ok {
		t.Error("expected 'nonexistent_relation_xyz' to not be canonical")
	}
}
