package index_test

import (
	"context"
	"testing"
)

// TestDeleteTenant purges a tenant and asserts every table is cleared — notably
// note_versions, whose append-only trigger must yield to the app.purge escape
// hatch (migration 008) so the history is actually removed.
func TestDeleteTenant(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	seedMarkdown(t, store, tenantID, "a.md", "# A\n\nalpha gamma", false)
	seedMarkdown(t, store, tenantID, "b.md", "# B\n\nbeta gamma", false)

	res, err := store.DeleteTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	if res.Notes != 2 {
		t.Errorf("deleted %d notes, want 2", res.Notes)
	}
	if res.Versions < 2 {
		t.Errorf("deleted %d versions, want >= 2 (append-only history not purged)", res.Versions)
	}

	// Nothing is retrievable, and the rows are gone.
	if hits := searchIDs(t, store, tenantID, "alpha beta gamma"); len(hits) != 0 {
		t.Errorf("search after purge returned %v, want none", hits)
	}
	if _, found, err := store.NoteState(ctx, tenantID, "syn:a"); err != nil || found {
		t.Errorf("note syn:a survived purge (found=%v err=%v)", found, err)
	}
	if vers, err := store.NoteVersions(ctx, tenantID, "syn:a"); err != nil || len(vers) != 0 {
		t.Errorf("history for syn:a survived purge: %d rows (err %v)", len(vers), err)
	}
}
