package store_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestRecordAndQuery(t *testing.T) {
	base, tenantID := setupTestBase(t)
	as := store.NewAuditStore(base)
	ctx := context.Background()

	err := as.RecordAudit(ctx, tenantID, "create", "node", "test-node-1", "test-actor",
		map[string]any{"reason": "testing"})
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	entries, hasMore, err := as.QueryAudit(ctx, tenantID, models.AuditQueryOpts{
		EntityType: "node",
		EntityID:   "test-node-1",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("QueryAudit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("QueryAudit returned %d entries, want 1", len(entries))
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}

	e := entries[0]
	if e.Action != "create" {
		t.Errorf("Action = %q, want %q", e.Action, "create")
	}
	if e.Actor != "test-actor" {
		t.Errorf("Actor = %q, want %q", e.Actor, "test-actor")
	}
	if e.Detail["reason"] != "testing" {
		t.Errorf("Detail[reason] = %v, want testing", e.Detail["reason"])
	}
}

func TestPurgeOldEntries(t *testing.T) {
	base, tenantID := setupTestBase(t)
	as := store.NewAuditStore(base)
	ctx := context.Background()

	// Insert an entry then backdate it via raw SQL.
	err := as.RecordAudit(ctx, tenantID, "delete", "node", "old-node", "", nil)
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	env := getTestEnv(t)
	tx, txErr := env.pool.Begin(ctx)
	if txErr != nil {
		t.Fatalf("begin tx: %v", txErr)
	}
	if _, txErr = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); txErr != nil {
		t.Fatalf("set tenant: %v", txErr)
	}
	if _, txErr = tx.Exec(ctx,
		"UPDATE kg_audit_log SET created_at = NOW() - INTERVAL '400 days' WHERE tenant_id = $1 AND entity_id = 'old-node'",
		tenantID); txErr != nil {
		t.Fatalf("backdating audit entry: %v", txErr)
	}
	if txErr = tx.Commit(ctx); txErr != nil {
		t.Fatalf("commit backdate: %v", txErr)
	}

	// Also insert a recent entry that should NOT be purged.
	err = as.RecordAudit(ctx, tenantID, "create", "node", "new-node", "", nil)
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	purged, err := as.PurgeOldEntries(ctx, tenantID, 365)
	if err != nil {
		t.Fatalf("PurgeOldEntries: %v", err)
	}

	if purged < 1 {
		t.Errorf("PurgeOldEntries purged %d, want >= 1", purged)
	}

	// Verify the recent entry still exists.
	entries, _, err := as.QueryAudit(ctx, tenantID, models.AuditQueryOpts{
		EntityID: "new-node",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("QueryAudit after purge: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("QueryAudit after purge = %d entries, want 1", len(entries))
	}
}
