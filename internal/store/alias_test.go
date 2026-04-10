package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestAliasStore_CRUDAndList(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	as := store.NewAliasStore(base)
	ctx := context.Background()

	nodeReq := models.CreateNodeRequest{Type: "person", Label: "William Gates"}
	_ = nodeReq.Validate()
	node, err := ns.CreateNode(ctx, tenantID, nodeReq)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	confidence := 0.9
	created, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{
		NodeID:     node.ID,
		Alias:      "  BILL   GATES ",
		AliasType:  "nickname",
		Confidence: &confidence,
		Source:     "test",
	})
	if err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	if created.NormalizedAlias != "bill gates" {
		t.Fatalf("NormalizedAlias = %q, want %q", created.NormalizedAlias, "bill gates")
	}

	got, err := as.GetAlias(ctx, tenantID, created.ID.String())
	if err != nil {
		t.Fatalf("GetAlias: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("GetAlias ID = %q, want %q", got.ID, created.ID)
	}

	if _, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{
		NodeID:    node.ID,
		Alias:     "bill gates",
		AliasType: "nickname",
		Source:    "dupe",
	}); !errors.Is(err, models.ErrDuplicateKey) {
		t.Fatalf("CreateAlias duplicate err = %v, want ErrDuplicateKey", err)
	}

	if _, err := as.CreateAlias(ctx, tenantID, models.CreateAliasRequest{
		NodeID:    node.ID,
		Alias:     "William H. Gates",
		AliasType: "full_name",
		Source:    "test",
	}); err != nil {
		t.Fatalf("CreateAlias second: %v", err)
	}

	aliases, hasMore, err := as.ListAliases(ctx, tenantID, models.AliasListOpts{NodeID: node.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListAliases by node: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("ListAliases len = %d, want 2", len(aliases))
	}
	if hasMore {
		t.Fatal("hasMore = true, want false")
	}

	filtered, hasMore, err := as.ListAliases(ctx, tenantID, models.AliasListOpts{
		NormalizedAlias: " bill   gates ",
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("ListAliases by normalized alias: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != created.ID {
		t.Fatalf("filtered[0].ID = %q, want %q", filtered[0].ID, created.ID)
	}
	if hasMore {
		t.Fatal("filtered hasMore = true, want false")
	}

	if err := as.DeleteAlias(ctx, tenantID, created.ID.String()); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if _, err := as.GetAlias(ctx, tenantID, created.ID.String()); !errors.Is(err, models.ErrAliasNotFound) {
		t.Fatalf("GetAlias after delete err = %v, want ErrAliasNotFound", err)
	}
}
