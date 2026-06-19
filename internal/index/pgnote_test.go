package index_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// newStoreTest connects to TEST_DATABASE_URL (skipping when unset) and returns a
// Store, the underlying pool (for out-of-band SQL the public API does not
// expose), and a random tenant. The schema is assumed migrated (the loop gate
// runs the fresh-migration step first). Cleanup removes the tenant's
// notes/chunks/sources; note_versions is append-only (a trigger blocks DELETE),
// but each test uses a fresh random tenant so leftover history rows are
// invisible to every other tenant.
func newStoreTest(t *testing.T) (*index.Store, *dbpool.Pool, string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := dbpool.NewPool(ctx, dbURL, 4)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)

	tenantID := uuid.New().String()
	t.Cleanup(func() { cleanupTenant(pool, tenantID) })

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	return index.NewStore(pool, log), pool, tenantID
}

// cleanupTenant best-effort deletes a tenant's live rows after a test.
func cleanupTenant(pool *dbpool.Pool, tenantID string) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return
	}
	_, _ = tx.Exec(ctx, "DELETE FROM chunks WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_, _ = tx.Exec(ctx, "DELETE FROM notes WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_, _ = tx.Exec(ctx, "DELETE FROM sources WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_ = tx.Commit(ctx)
}

func TestPGNote_Lifecycle(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:big-jerry"

	// Create.
	res, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: id, Tier: "tail", Title: "Big Jerry", Body: "a mature whitetail buck on the ridge", Surface: "laptop",
	}, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Version != 1 || res.Op != "create" {
		t.Fatalf("create result = %+v, want version 1 op create", res)
	}
	if !contains(searchIDs(t, store, tenant, "whitetail"), id) {
		t.Fatal("FTS did not return the created note")
	}

	// Update (un-supersede content; new body, expectedVersion 1).
	res, err = store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: id, Tier: "tail", Title: "Big Jerry", Body: "a wary doe at the creek", Surface: "phone",
	}, 1)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.Version != 2 || res.Op != "update" {
		t.Fatalf("update result = %+v, want version 2 op update", res)
	}
	if contains(searchIDs(t, store, tenant, "whitetail"), id) {
		t.Fatal("FTS still returns the stale body after update")
	}
	if !contains(searchIDs(t, store, tenant, "doe"), id) {
		t.Fatal("FTS did not return the updated body")
	}

	// Delete (tombstone, expectedVersion 2).
	res, err = store.DeleteNote(ctx, tenant, id, 2, "phone")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Version != 3 || res.Op != "delete" {
		t.Fatalf("delete result = %+v, want version 3 op delete", res)
	}
	if contains(searchIDs(t, store, tenant, "doe"), id) {
		t.Fatal("FTS returns a tombstoned note")
	}
	st, found, err := store.NoteState(ctx, tenant, id)
	if err != nil {
		t.Fatalf("note state: %v", err)
	}
	if !found || !st.Deleted {
		t.Fatalf("after delete: found=%v deleted=%v, want found+deleted", found, st.Deleted)
	}
	if recs, _ := store.LoadNotes(ctx, tenant, []string{id}); len(recs) != 0 {
		t.Fatalf("LoadNotes returned a tombstoned note: %+v", recs)
	}

	// Restore the latest non-delete version (the doe update), expectedVersion 3.
	res, err = store.RestoreNote(ctx, tenant, id, 0, 3, "laptop")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Version != 4 || res.Op != "restore" {
		t.Fatalf("restore result = %+v, want version 4 op restore", res)
	}
	if !contains(searchIDs(t, store, tenant, "doe"), id) {
		t.Fatal("FTS did not return the restored note")
	}

	// Full history: create, update, delete, restore.
	versions, err := store.NoteVersions(ctx, tenant, id)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	gotOps := make([]string, len(versions))
	for i, v := range versions {
		gotOps[i] = v.Op
	}
	wantOps := []string{"create", "update", "delete", "restore"}
	if len(gotOps) != len(wantOps) {
		t.Fatalf("history ops = %v, want %v", gotOps, wantOps)
	}
	for i := range wantOps {
		if gotOps[i] != wantOps[i] || versions[i].Version != i+1 {
			t.Fatalf("history[%d] = (v%d %s), want (v%d %s)", i, versions[i].Version, versions[i].Op, i+1, wantOps[i])
		}
	}
}

func TestPGNote_OptimisticConcurrency(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:concurrent"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "first"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Re-creating an existing id (expectedVersion 0) must conflict, reporting the
	// real current version.
	_, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "clobber"}, 0)
	var conflict *index.VersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("re-create: want VersionConflictError, got %v", err)
	}
	if conflict.Actual != 1 {
		t.Fatalf("conflict.Actual = %d, want 1", conflict.Actual)
	}

	// A stale update (expectedVersion 99) must conflict, not overwrite.
	_, err = store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "stale"}, 99)
	if !errors.As(err, &conflict) {
		t.Fatalf("stale update: want VersionConflictError, got %v", err)
	}
	st, _, _ := store.NoteState(ctx, tenant, id)
	if st.Body != "first" || st.Version != 1 {
		t.Fatalf("after rejected writes: body=%q version=%d, want first/1", st.Body, st.Version)
	}
}

func TestPGNote_DeleteNotFound(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	var notFound *index.NoteNotFoundError
	if _, err := store.DeleteNote(ctx, tenant, "test:ghost", 0, ""); !errors.As(err, &notFound) {
		t.Fatalf("delete missing: want NoteNotFoundError, got %v", err)
	}

	const id = "test:dead"
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "x"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.DeleteNote(ctx, tenant, id, 1, ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Deleting an already-tombstoned note: there is no live note to delete.
	if _, err := store.DeleteNote(ctx, tenant, id, 2, ""); !errors.As(err, &notFound) {
		t.Fatalf("delete tombstone: want NoteNotFoundError, got %v", err)
	}
}

func TestReindexPGNative_RebuildsChunks(t *testing.T) {
	store, pool, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:reindex"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "rebuildable elk sign"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Simulate index drift: drop the chunk projection out-of-band.
	execTenant(t, pool, tenant, "DELETE FROM chunks WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	if contains(searchIDs(t, store, tenant, "elk"), id) {
		t.Fatal("expected no FTS hit after chunks dropped")
	}

	n, err := store.ReindexPGNative(ctx, tenant)
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if n != 1 {
		t.Fatalf("reindexed %d notes, want 1", n)
	}
	if !contains(searchIDs(t, store, tenant, "elk"), id) {
		t.Fatal("FTS did not return the note after PG-to-PG reindex")
	}
}

func TestNoteVersions_TenantIsolation(t *testing.T) {
	store, pool, tenantA := newStoreTest(t)
	tenantB := uuid.New().String()
	t.Cleanup(func() { cleanupTenant(pool, tenantB) })
	ctx := context.Background()
	const id = "test:secret"

	if _, err := store.WriteNote(ctx, tenantA, &index.PGNoteInput{ID: id, Title: "Secret", Body: "tenant A only"}, 0); err != nil {
		t.Fatalf("A create: %v", err)
	}

	// Tenant B sees none of A's data through any read path.
	if vs, err := store.NoteVersions(ctx, tenantB, id); err != nil || len(vs) != 0 {
		t.Fatalf("B NoteVersions = (%d, %v), want (0, nil)", len(vs), err)
	}
	if _, found, err := store.NoteState(ctx, tenantB, id); err != nil || found {
		t.Fatalf("B NoteState found=%v err=%v, want not-found", found, err)
	}
	if hits := searchIDs(t, store, tenantB, "Secret"); len(hits) != 0 {
		t.Fatalf("B search leaked A's note: %v", hits)
	}

	// Tenant A still sees its own history.
	if vs, err := store.NoteVersions(ctx, tenantA, id); err != nil || len(vs) != 1 {
		t.Fatalf("A NoteVersions = (%d, %v), want (1, nil)", len(vs), err)
	}

	// RLS WITH CHECK must block B from writing a version row into A's tenant.
	if err := tryCrossTenantVersionInsert(pool, tenantB, tenantA); err == nil {
		t.Fatal("cross-tenant note_versions insert succeeded; RLS WITH CHECK not enforced")
	}
}

// searchIDs runs a default search and returns the hit note ids.
func searchIDs(t *testing.T, store *index.Store, tenant, query string) []string {
	t.Helper()
	hits, err := store.SearchNotes(context.Background(), tenant, query, index.SearchOpts{})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return ids
}

// execTenant runs a statement in a committed tenant-scoped transaction.
func execTenant(t *testing.T, pool *dbpool.Pool, tenant, sql string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	if _, err := tx.Exec(ctx, sql); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// tryCrossTenantVersionInsert attempts to insert a note_versions row for
// rowTenant while the session is scoped to gucTenant; RLS WITH CHECK should
// reject it. Returns the resulting error (nil means the bleed was NOT blocked).
func tryCrossTenantVersionInsert(pool *dbpool.Pool, gucTenant, rowTenant string) error {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", gucTenant); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO note_versions (tenant_id, note_id, version, op) VALUES ($1, 'evil', 1, 'create')`,
		rowTenant)
	return err
}

// contains reports whether ids includes target.
func contains(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
