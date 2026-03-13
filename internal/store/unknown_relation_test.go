package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestLogUnknownRelation(t *testing.T) {
	base, tenantID := setupTestBase(t)
	urs := store.NewUnknownRelationStore(base)
	ctx := context.Background()

	// First log creates the record.
	err := urs.LogUnknownRelation(ctx, tenantID, "MENTORS", "Alice", "Bob", "Alice mentors Bob")
	if err != nil {
		t.Fatalf("LogUnknownRelation: %v", err)
	}

	// Second log should increment count.
	err = urs.LogUnknownRelation(ctx, tenantID, "MENTORS", "Alice", "Bob", "Alice mentors Bob again")
	if err != nil {
		t.Fatalf("LogUnknownRelation (upsert): %v", err)
	}

	// List and verify count was incremented.
	results, err := urs.ListUnknownRelations(ctx, tenantID, models.UnknownRelationListOpts{
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnknownRelations: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Count != 2 {
		t.Errorf("count = %d, want 2", results[0].Count)
	}

	if results[0].RelationType != "MENTORS" {
		t.Errorf("relation_type = %q, want %q", results[0].RelationType, "MENTORS")
	}
}

func TestResolveUnknownRelation(t *testing.T) {
	base, tenantID := setupTestBase(t)
	urs := store.NewUnknownRelationStore(base)
	ctx := context.Background()

	err := urs.LogUnknownRelation(ctx, tenantID, "MENTORS", "Alice", "Bob", "source text")
	if err != nil {
		t.Fatalf("LogUnknownRelation: %v", err)
	}

	// Get the relation to find its ID.
	results, err := urs.ListUnknownRelations(ctx, tenantID, models.UnknownRelationListOpts{
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnknownRelations: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	id := results[0].ID.String()

	// Resolve it.
	err = urs.ResolveUnknownRelation(ctx, tenantID, id, "TEACHES")
	if err != nil {
		t.Fatalf("ResolveUnknownRelation: %v", err)
	}

	// Should no longer appear in unresolved list.
	unresolved, err := urs.ListUnknownRelations(ctx, tenantID, models.UnknownRelationListOpts{
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnknownRelations after resolve: %v", err)
	}

	if len(unresolved) != 0 {
		t.Errorf("got %d unresolved, want 0", len(unresolved))
	}

	// Should appear in resolved list.
	resolved, err := urs.ListUnknownRelations(ctx, tenantID, models.UnknownRelationListOpts{
		ResolvedOnly: true,
		Limit:        50,
	})
	if err != nil {
		t.Fatalf("ListUnknownRelations resolved: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("got %d resolved, want 1", len(resolved))
	}

	if resolved[0].ResolvedAs == nil || *resolved[0].ResolvedAs != "TEACHES" {
		t.Errorf("resolved_as = %v, want %q", resolved[0].ResolvedAs, "TEACHES")
	}
}

func TestResolveUnknownRelation_NotFound(t *testing.T) {
	base, tenantID := setupTestBase(t)
	urs := store.NewUnknownRelationStore(base)
	ctx := context.Background()

	err := urs.ResolveUnknownRelation(ctx, tenantID, "00000000-0000-0000-0000-000000000000", "TEACHES")
	if err == nil {
		t.Fatal("expected error for nonexistent relation")
	}

	if !errors.Is(err, models.ErrUnknownRelationNotFound) {
		t.Errorf("got %v, want ErrUnknownRelationNotFound", err)
	}
}
