package index_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// newStoreTest connects to TEST_DATABASE_URL (skipping when unset) and returns a
// Store, the underlying pool (for out-of-band SQL the public API does not
// expose), and a random tenant. The schema is assumed migrated (the loop gate
// runs the fresh-migration step first). Cleanup removes the tenant's
// notes/chunks; note_versions is append-only (a trigger blocks DELETE), but each
// test uses a fresh random tenant so leftover history rows are invisible to
// every other tenant.
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

// TestPGNote_ConcurrentCreateConflict exercises the create-create race
// deterministically: a create (expectedVersion 0) locks no row, so it cannot see
// a concurrent creator and only trips the note_versions PK as a unique violation.
// Pre-seeding a version-1 history row for an id with no live note reproduces
// exactly the loser's state, and WriteNote must report it as a clean
// *VersionConflictError (409), not the raw unique violation (500).
func TestPGNote_ConcurrentCreateConflict(t *testing.T) {
	store, pool, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:race"

	// The winning creator already wrote version 1 to history; the live notes row
	// is irrelevant to the loser's failure, which happens at appendVersion.
	execTenant(t, pool, tenant,
		"INSERT INTO note_versions (tenant_id, note_id, version, op) "+
			"VALUES (current_setting('app.tenant_id')::uuid, 'test:race', 1, 'create')")

	_, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "loser"}, 0)
	var conflict *index.VersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("concurrent create: want VersionConflictError, got %v", err)
	}
	if conflict.Expected != 0 || conflict.Actual != 1 {
		t.Fatalf("conflict = (expected %d, actual %d), want (0, 1)", conflict.Expected, conflict.Actual)
	}
}

// TestPGNote_DeleteRestoreConflicts covers the optimistic-concurrency guards on
// the delete and restore paths, which the lifecycle/not-found tests never reach.
func TestPGNote_DeleteRestoreConflicts(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:conflict"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "live"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}

	var conflict *index.VersionConflictError
	// Delete with a stale expectedVersion on a LIVE note hits the version check.
	if _, err := store.DeleteNote(ctx, tenant, id, 99, "x"); !errors.As(err, &conflict) {
		t.Fatalf("delete stale: want VersionConflictError, got %v", err)
	} else if conflict.Actual != 1 {
		t.Fatalf("delete conflict.Actual = %d, want 1", conflict.Actual)
	}

	// Tombstone it for real, then exercise restore's guards.
	if _, err := store.DeleteNote(ctx, tenant, id, 1, "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Restore with a stale expectedVersion (current is 2 after the delete).
	if _, err := store.RestoreNote(ctx, tenant, id, 0, 99, "x"); !errors.As(err, &conflict) {
		t.Fatalf("restore stale: want VersionConflictError, got %v", err)
	}
	// Restore a never-existed id is not-found.
	var notFound *index.NoteNotFoundError
	if _, err := store.RestoreNote(ctx, tenant, "test:ghost", 0, 0, "x"); !errors.As(err, &notFound) {
		t.Fatalf("restore missing: want NoteNotFoundError, got %v", err)
	}
}

// TestNoteVersions_AppendOnly verifies the database trigger makes the audit log
// immutable: UPDATE and DELETE on note_versions are rejected even by the owning
// tenant. Only INSERT (a new version, including a restore) is legal.
func TestNoteVersions_AppendOnly(t *testing.T) {
	store, pool, tenant := newStoreTest(t)
	ctx := context.Background()
	const id = "test:immutable"

	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{ID: id, Body: "history"}, 0); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := tryTenantExec(pool, tenant,
		"UPDATE note_versions SET body = 'tamper' WHERE note_id = 'test:immutable'"); err == nil {
		t.Fatal("UPDATE on note_versions succeeded; append-only trigger not enforced")
	}
	if err := tryTenantExec(pool, tenant,
		"DELETE FROM note_versions WHERE note_id = 'test:immutable'"); err == nil {
		t.Fatal("DELETE on note_versions succeeded; append-only trigger not enforced")
	}
	// TRUNCATE is statement-level — the row trigger never sees it — so the
	// BEFORE TRUNCATE guard (migration 006) must reject it too. (The guard
	// aborts the statement, so this never actually wipes the shared test table.)
	if err := tryTenantExec(pool, tenant, "TRUNCATE note_versions"); err == nil {
		t.Fatal("TRUNCATE on note_versions succeeded; append-only TRUNCATE guard not enforced")
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
	hits, err := store.SearchNotes(context.Background(), tenant, query, &index.SearchOpts{})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	ids := make([]string, len(hits))
	for i := range hits {
		ids[i] = hits[i].ID
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

// tryTenantExec runs a statement in a tenant-scoped transaction and returns its
// error (rolling back), for asserting that a statement is REJECTED. nil means
// the statement was allowed.
func tryTenantExec(pool *dbpool.Pool, tenant, sql string) error {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, sql)
	return err
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
