package store_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestBoostNode(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ss := store.NewSalienceStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "concept", Label: "Boost Target"}
	_ = req.Validate()

	created, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	initialSalience := created.Salience

	boosted, err := ss.BoostNode(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("BoostNode: %v", err)
	}

	if !boosted.UserBoosted {
		t.Error("UserBoosted = false after boost")
	}
	if boosted.Salience <= initialSalience {
		t.Errorf("Salience after boost (%f) should be > initial (%f)", boosted.Salience, initialSalience)
	}
}

func TestRecalculateSalience(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ss := store.NewSalienceStore(base)
	ctx := context.Background()

	// Create a few nodes.
	for _, label := range []string{"Recalc A", "Recalc B"} {
		req := models.CreateNodeRequest{Type: "concept", Label: label}
		_ = req.Validate()
		if _, err := ns.CreateNode(ctx, tenantID, req); err != nil {
			t.Fatalf("CreateNode(%s): %v", label, err)
		}
	}

	count, err := ss.RecalculateSalience(ctx, tenantID)
	if err != nil {
		t.Fatalf("RecalculateSalience: %v", err)
	}

	// count may be 0 if scores already match formula; that's OK.
	// Just verify it doesn't error.
	t.Logf("RecalculateSalience updated %d nodes", count)
}
