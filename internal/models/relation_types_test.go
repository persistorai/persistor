package models_test

import (
	"errors"
	"testing"

	"github.com/persistorai/persistor/internal/models"
)

func TestIsCanonicalRelation(t *testing.T) {
	tests := []struct {
		name string
		rel  string
		want bool
	}{
		{"canonical created", "created", true},
		{"canonical uses", "uses", true},
		{"canonical depends_on", "depends_on", true},
		{"not canonical", "foobar", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := models.IsCanonicalRelation(tt.rel); got != tt.want {
				t.Errorf("IsCanonicalRelation(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestListRelationTypes(t *testing.T) {
	types := models.ListRelationTypes()

	// Should include at least the 37 canonical types.
	if len(types) < 37 {
		t.Errorf("ListRelationTypes() returned %d types, want >= 37", len(types))
	}

	// Verify sorted order.
	for i := 1; i < len(types); i++ {
		if types[i].Name <= types[i-1].Name {
			t.Errorf("not sorted: %q after %q", types[i].Name, types[i-1].Name)
		}
	}
}

func TestAddRelationType(t *testing.T) {
	// Use a unique name unlikely to conflict with other tests.
	name := "test_add_relation_unique_abc123"

	err := models.AddRelationType(name, "A test relation")
	if err != nil {
		t.Fatalf("AddRelationType: unexpected error: %v", err)
	}

	// Verify it appears in the list.
	found := false
	for _, rt := range models.ListRelationTypes() {
		if rt.Name == name {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("added type %q not found in ListRelationTypes()", name)
	}
}

func TestAddRelationType_EmptyName(t *testing.T) {
	err := models.AddRelationType("", "some description")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestAddRelationType_EmptyDescription(t *testing.T) {
	err := models.AddRelationType("test_empty_desc_xyz", "")
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestAddRelationType_DuplicateCanonical(t *testing.T) {
	err := models.AddRelationType("created", "duplicate of canonical")
	if !errors.Is(err, models.ErrDuplicateKey) {
		t.Errorf("expected ErrDuplicateKey, got: %v", err)
	}
}

func TestAddRelationType_DuplicateCustom(t *testing.T) {
	name := "test_dup_custom_xyz789"

	// First add might succeed or fail if a previous test run added it.
	_ = models.AddRelationType(name, "first")

	err := models.AddRelationType(name, "second")
	if !errors.Is(err, models.ErrDuplicateKey) {
		t.Errorf("expected ErrDuplicateKey, got: %v", err)
	}
}
