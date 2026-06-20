package index_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// TestTenantBleed_Notes is the load-bearing multi-tenant isolation check across
// the notes table and every read/write path, plus the raw RLS policies. It is
// only meaningful against a NON-SUPERUSER, NON-BYPASSRLS role — a superuser or
// BYPASSRLS role silently ignores even FORCE'd RLS — so it asserts that
// precondition first (dbpool.NewPool also refuses to start otherwise).
func TestTenantBleed_Notes(t *testing.T) {
	store, pool, tenantA := newStoreTest(t)
	requireNonSuperuser(t, pool)

	tenantB := uuid.New().String()
	t.Cleanup(func() { cleanupTenant(pool, tenantB) })
	ctx := context.Background()
	const id = "test:bleed"

	if _, err := store.WriteNote(ctx, tenantA, &index.PGNoteInput{
		ID: id, Title: "A secret", Body: "tenant A only body", Surface: "test",
	}, 0); err != nil {
		t.Fatalf("A create: %v", err)
	}

	// 1) B cannot READ A's note through any path.
	if notes, err := store.ExportNotes(ctx, tenantB); err != nil || len(notes) != 0 {
		t.Fatalf("B ExportNotes = (%d, %v), want (0, nil) — export leaked across tenants", len(notes), err)
	}
	if _, found, err := store.NoteState(ctx, tenantB, id); err != nil || found {
		t.Fatalf("B NoteState found=%v err=%v, want not-found", found, err)
	}
	if hits := searchIDs(t, store, tenantB, "secret"); len(hits) != 0 {
		t.Fatalf("B search leaked A's note: %v", hits)
	}

	// 2) B "deleting" A's note must be a not-found, never a cross-tenant delete.
	if _, err := store.DeleteNote(ctx, tenantB, id, 1, "test"); err == nil {
		t.Fatal("B DeleteNote on A's id succeeded; expected not-found")
	}

	// 3) B writing the same id creates B's OWN note (PK is tenant_id+id) and must
	// not touch A's row.
	if _, err := store.WriteNote(ctx, tenantB, &index.PGNoteInput{
		ID: id, Title: "B note", Body: "tenant B body", Surface: "test",
	}, 0); err != nil {
		t.Fatalf("B create same id: %v", err)
	}
	st, found, err := store.NoteState(ctx, tenantA, id)
	if err != nil || !found {
		t.Fatalf("A NoteState after B write: found=%v err=%v", found, err)
	}
	if st.Body != "tenant A only body" {
		t.Fatalf("A's note body mutated by B: %q", st.Body)
	}

	// 4) Raw RLS: with the GUC set to B, a direct INSERT into notes claiming A's
	// tenant must be blocked by WITH CHECK.
	if err := tryCrossTenantNoteInsert(pool, tenantB, tenantA); err == nil {
		t.Fatal("cross-tenant notes INSERT succeeded; RLS WITH CHECK not enforced")
	}
	// 5) Raw RLS: with the GUC set to B, an UPDATE targeting A's rows must affect
	// zero rows (USING hides them) — A's body stays intact.
	if n := tryCrossTenantNoteUpdate(t, pool, tenantB, tenantA); n != 0 {
		t.Fatalf("cross-tenant notes UPDATE affected %d rows; RLS USING not enforced", n)
	}
}

// requireNonSuperuser fails the test unless the connected role is NOSUPERUSER and
// NOBYPASSRLS — the precondition that makes RLS, and these bleed assertions, mean
// anything.
func requireNonSuperuser(t *testing.T, pool *dbpool.Pool) {
	t.Helper()
	var super, bypass bool
	if err := pool.QueryRow(context.Background(),
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&super, &bypass); err != nil {
		t.Fatalf("checking role privileges: %v", err)
	}
	if super || bypass {
		t.Fatalf("test role is superuser=%v bypassrls=%v; RLS would be silently voided", super, bypass)
	}
}

// tryCrossTenantNoteInsert sets app.tenant_id=gucTenant then inserts a notes row
// stamped with rowTenant; RLS WITH CHECK should reject it. Returns the error (nil
// means the bleed was NOT blocked).
func tryCrossTenantNoteInsert(pool *dbpool.Pool, gucTenant, rowTenant string) error {
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
		`INSERT INTO notes (id, tenant_id, title, body) VALUES ('evil', $1, 'x', 'y')`,
		rowTenant)
	return err
}

// tryCrossTenantNoteUpdate sets app.tenant_id=gucTenant then tries to mutate
// rowTenant's notes; RLS USING should make this affect zero rows. Returns the
// number of rows affected.
func tryCrossTenantNoteUpdate(t *testing.T, pool *dbpool.Pool, gucTenant, rowTenant string) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", gucTenant); err != nil {
		t.Fatalf("set guc: %v", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE notes SET body = 'pwned' WHERE tenant_id = $1`, rowTenant)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	return tag.RowsAffected()
}
