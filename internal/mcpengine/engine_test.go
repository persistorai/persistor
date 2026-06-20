package mcpengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpengine"
)

// newTestStore connects to TEST_DATABASE_URL (skipping when unset) and returns a
// PG-native store plus a fresh tenant id. The schema is assumed migrated (the
// loop gate runs the fresh-migration step first).
func newTestStore(t *testing.T) (store *index.Store, tenantID string) {
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

	tenantID = uuid.New().String()
	t.Cleanup(func() { cleanupTenant(t, pool, tenantID) })

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store = index.NewStore(pool, log)
	return store, tenantID
}

func cleanupTenant(t *testing.T, pool *dbpool.Pool, tenantID string) {
	t.Helper()
	clean := context.Background()
	tx, err := pool.Begin(clean)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(clean) }()
	if _, err := tx.Exec(clean, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return
	}
	_, _ = tx.Exec(clean, "DELETE FROM chunks WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_, _ = tx.Exec(clean, "DELETE FROM note_versions WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_, _ = tx.Exec(clean, "DELETE FROM notes WHERE tenant_id = current_setting('app.tenant_id')::uuid")
	_ = tx.Commit(clean)
}

// newTestEngine returns an Engine over a freshly-seeded tenant: one Core note
// (SOUL) and one Tail note (the Aurora protocol). It also returns the store so a
// test can build a second engine for a different tenant.
func newTestEngine(t *testing.T) (*mcpengine.Engine, *index.Store) {
	t.Helper()
	store, tenantID := newTestStore(t)
	ctx := context.Background()
	seed(t, store, tenantID, &index.PGNoteInput{
		ID: "scout:soul", Tier: "core", Title: "Soul",
		Body: "We are the Northwind expedition crew.", Surface: "test",
	})
	seed(t, store, tenantID, &index.PGNoteInput{
		ID: "scout:memory-daily-aurora", Title: "Aurora Protocol",
		Body: "The safety protocol for polar storms.", Surface: "test",
	})
	if _, err := store.ReconcileSupersessions(ctx, tenantID); err != nil {
		t.Fatalf("seed reconcile: %v", err)
	}
	return mcpengine.NewEngine(store, tenantID, mcpengine.WithSurface("test")), store
}

func seed(t *testing.T, store *index.Store, tenantID string, in *index.PGNoteInput) {
	t.Helper()
	if _, err := store.WriteNote(context.Background(), tenantID, in, 0); err != nil {
		t.Fatalf("seed %s: %v", in.ID, err)
	}
}

// TestEngine_RoundTrip exercises all four read/write tool backends against the DB.
func TestEngine_RoundTrip(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	// search: finds the Aurora note.
	sr, err := e.Search(ctx, mcpengine.SearchInput{Query: "polar storm protocol"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !hasResult(sr.Results, "scout:memory-daily-aurora") {
		t.Fatalf("search missed aurora: %+v", sr.Results)
	}

	// get: returns the full body and the current version.
	gr, err := e.Get(ctx, mcpengine.GetInput{ID: "scout:memory-daily-aurora"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !gr.Found || gr.Body == "" || gr.Version != 1 {
		t.Fatalf("get found=%v body=%q version=%d", gr.Found, gr.Body, gr.Version)
	}

	// write: add a new note that supersedes the Aurora protocol.
	wr, err := e.Write(ctx, &mcpengine.WriteInput{
		ID: "scout:aurora-v2", Supersedes: "scout:memory-daily-aurora",
		Body: "# Aurora Protocol v2\n\nUpdated polar storm protocol.",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if wr.ID != "scout:aurora-v2" || wr.Op != "create" || wr.Version != 1 || wr.Superseded != 1 {
		t.Fatalf("write = %+v, want id=scout:aurora-v2 op=create version=1 superseded=1", wr)
	}

	// search again: stale note hidden, correction surfaces.
	sr, err = e.Search(ctx, mcpengine.SearchInput{Query: "polar storm protocol"})
	if err != nil {
		t.Fatalf("search 2: %v", err)
	}
	if hasResult(sr.Results, "scout:memory-daily-aurora") {
		t.Errorf("superseded note still in results: %+v", sr.Results)
	}
	if !hasResult(sr.Results, "scout:aurora-v2") {
		t.Errorf("correction missing: %+v", sr.Results)
	}

	// brief: Core (the SOUL note) is always present.
	br, err := e.Brief(ctx, mcpengine.BriefInput{Seed: "polar storm"})
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if br.TotalTokens == 0 || br.Markdown == "" {
		t.Errorf("brief empty: %+v", br)
	}
}

// TestEngine_WriteDerivesIDFromPath: with no explicit id, the id is derived from
// the path slug.
func TestEngine_WriteDerivesIDFromPath(t *testing.T) {
	e, _ := newTestEngine(t)
	wr, err := e.Write(context.Background(), &mcpengine.WriteInput{
		Path: "memory/widget.md", Body: "# Widget\n\nbody",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if wr.ID != "memory-widget" {
		t.Fatalf("derived id = %q, want memory-widget", wr.ID)
	}
}

// TestEngine_NamespaceFilter: writes land in a namespace, search can restrict to
// one, get can guard on one, and an unspecified namespace defaults to "default".
func TestEngine_NamespaceFilter(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "scout:n", Namespace: "scout", Body: "shared keyword alpha scout"}); err != nil {
		t.Fatalf("scout write: %v", err)
	}
	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "work:n", Namespace: "work", Body: "shared keyword alpha work"}); err != nil {
		t.Fatalf("work write: %v", err)
	}

	// Search restricted to the scout namespace returns only its note.
	sr, err := e.Search(ctx, mcpengine.SearchInput{Query: "shared keyword alpha", Namespace: "scout"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !hasResult(sr.Results, "scout:n") || hasResult(sr.Results, "work:n") {
		t.Fatalf("namespace-filtered search = %+v, want only scout:n", sr.Results)
	}

	// Get with a mismatched namespace guard reads as not-found.
	if gr, _ := e.Get(ctx, mcpengine.GetInput{ID: "scout:n", Namespace: "work"}); gr.Found {
		t.Fatal("get with wrong namespace should be not-found")
	}
	gr, err := e.Get(ctx, mcpengine.GetInput{ID: "scout:n", Namespace: "scout"})
	if err != nil || !gr.Found || gr.Namespace != "scout" {
		t.Fatalf("get scout:n = %+v (err %v), want namespace scout", gr, err)
	}

	// No namespace specified → default bucket.
	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "plain", Body: "no namespace"}); err != nil {
		t.Fatalf("default write: %v", err)
	}
	if gd, _ := e.Get(ctx, mcpengine.GetInput{ID: "plain"}); gd.Namespace != "default" {
		t.Fatalf("default namespace = %q, want default", gd.Namespace)
	}
}

// TestEngine_WriteOptimisticConcurrency: create needs version 0; updating an
// existing note needs its current version, and a stale version is a conflict.
func TestEngine_WriteOptimisticConcurrency(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	// Create.
	w1, err := e.Write(ctx, &mcpengine.WriteInput{ID: "occ:note", Body: "v1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w1.Version != 1 || w1.Op != "create" {
		t.Fatalf("create = %+v, want version=1 op=create", w1)
	}

	// A second create (expected_version 0) over the existing note is a conflict.
	_, err = e.Write(ctx, &mcpengine.WriteInput{ID: "occ:note", Body: "dup"})
	var conflict *index.VersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("re-create: got %v, want *VersionConflictError", err)
	}

	// Update with the right version succeeds and bumps the version.
	w2, err := e.Write(ctx, &mcpengine.WriteInput{ID: "occ:note", Body: "v2", ExpectedVersion: 1})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if w2.Version != 2 || w2.Op != "update" {
		t.Fatalf("update = %+v, want version=2 op=update", w2)
	}

	// Update with a stale version is a conflict.
	_, err = e.Write(ctx, &mcpengine.WriteInput{ID: "occ:note", Body: "stale", ExpectedVersion: 1})
	if !errors.As(err, &conflict) {
		t.Fatalf("stale update: got %v, want *VersionConflictError", err)
	}
}

// TestEngine_DeleteRestore: a delete tombstones a note (gone from get/search); a
// restore brings it back.
func TestEngine_DeleteRestore(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "dr:note", Body: "deletable polar body"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	gr, err := e.Get(ctx, mcpengine.GetInput{ID: "dr:note"})
	if err != nil || !gr.Found {
		t.Fatalf("get after create: found=%v err=%v", gr.Found, err)
	}

	del, err := e.Delete(ctx, mcpengine.DeleteInput{ID: "dr:note", ExpectedVersion: gr.Version})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if del.Op != "delete" {
		t.Fatalf("delete op = %q, want delete", del.Op)
	}
	if gr2, _ := e.Get(ctx, mcpengine.GetInput{ID: "dr:note"}); gr2.Found {
		t.Fatal("note still found after delete")
	}

	res, err := e.Restore(ctx, mcpengine.RestoreInput{ID: "dr:note", ExpectedVersion: del.Version})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Op != "restore" {
		t.Fatalf("restore op = %q, want restore", res.Op)
	}
	gr3, err := e.Get(ctx, mcpengine.GetInput{ID: "dr:note"})
	if err != nil || !gr3.Found || gr3.Body != "deletable polar body" {
		t.Fatalf("get after restore: found=%v body=%q err=%v", gr3.Found, gr3.Body, err)
	}
}

// TestEngine_WriteTenantIsolation: two engines over the same store, different
// tenants. The same id resolves to a separate note per tenant; neither write
// touches the other's row.
func TestEngine_WriteTenantIsolation(t *testing.T) {
	eA, store := newTestEngine(t)
	ctx := context.Background()

	tenantB := uuid.New().String()
	pool := poolFromStore(t)
	t.Cleanup(func() { cleanupTenant(t, pool, tenantB) })
	eB := mcpengine.NewEngine(store, tenantB, mcpengine.WithSurface("test-b"))

	if _, err := eA.Write(ctx, &mcpengine.WriteInput{ID: "shared:id", Body: "tenant A body"}); err != nil {
		t.Fatalf("A write: %v", err)
	}
	if _, err := eB.Write(ctx, &mcpengine.WriteInput{ID: "shared:id", Body: "tenant B body"}); err != nil {
		t.Fatalf("B write: %v", err)
	}

	grA, err := eA.Get(ctx, mcpengine.GetInput{ID: "shared:id"})
	if err != nil || grA.Body != "tenant A body" {
		t.Fatalf("A get = %q (err %v), want 'tenant A body' — B's write bled across", grA.Body, err)
	}
	grB, err := eB.Get(ctx, mcpengine.GetInput{ID: "shared:id"})
	if err != nil || grB.Body != "tenant B body" {
		t.Fatalf("B get = %q (err %v), want 'tenant B body'", grB.Body, err)
	}
}

// poolFromStore opens a second pool to the test DB for tenant-B cleanup (the
// store does not expose its pool).
func poolFromStore(t *testing.T) *dbpool.Pool {
	t.Helper()
	pool, err := dbpool.NewPool(context.Background(), os.Getenv("TEST_DATABASE_URL"), 2)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestMCPRoundTrip drives the server over an in-memory transport with a real MCP
// client, proving the wire protocol + schema inference work end to end through
// the shared NewServer constructor, including the new tool surface.
func TestMCPRoundTrip(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	server := mcpengine.NewServer(e, "test")

	serverTr, clientTr := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTr, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTr, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 6 {
		t.Fatalf("listed %d tools, want 6 (search/get/write/delete/restore/brief)", len(tools.Tools))
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": "polar storm protocol"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}

	var out mcpengine.SearchOutput
	reMarshal(t, res.StructuredContent, &out)
	if !hasResult(out.Results, "scout:memory-daily-aurora") {
		t.Errorf("MCP search missed aurora: %+v", out.Results)
	}
}

// TestEngine_WriteRejectsSelfSupersede: a write whose supersedes resolves to the
// same note it lands on is rejected, while superseding a different note works.
func TestEngine_WriteRejectsSelfSupersede(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "widget", Body: "# Widget\n\nOriginal."}); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Self-supersede (same id) must be rejected, not a silent no-op.
	_, err := e.Write(ctx, &mcpengine.WriteInput{ID: "widget", Supersedes: "widget", Body: "# Widget\n\nEdit.", ExpectedVersion: 1})
	var selfErr *index.SelfSupersedeError
	if !errors.As(err, &selfErr) {
		t.Errorf("self-supersede write: got %v, want *SelfSupersedeError", err)
	}

	// Superseding a DIFFERENT note is allowed and forks history.
	w, err := e.Write(ctx, &mcpengine.WriteInput{ID: "widget-v2", Supersedes: "widget", Body: "# Widget v2\n\nNew."})
	if err != nil {
		t.Fatalf("legitimate supersede write: %v", err)
	}
	if w.Superseded != 1 {
		t.Errorf("legitimate supersede: Superseded = %d, want 1", w.Superseded)
	}
}

func TestEngine_WriteRejectsMissingSupersedeTarget(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	// Superseding an id that does not exist must be rejected, not a silent write
	// with a dangling pointer (a prompt-injection memory-poisoning vector).
	_, err := e.Write(ctx, &mcpengine.WriteInput{
		ID: "ghost", Supersedes: "scout:does-not-exist", Body: "# Ghost\n\nbody",
	})
	if err == nil {
		t.Fatal("supersede of a non-existent note should error, got nil")
	}
}

// TestEngine_ReadOnlyRejectsWrite verifies a read-only engine denies the mutating
// tools before touching the store (so it needs no DB).
func TestEngine_ReadOnlyRejectsWrite(t *testing.T) {
	e := mcpengine.NewEngine(nil, "tenant", mcpengine.WithReadOnly(true))
	ctx := context.Background()
	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "x", Body: "# X\n\nbody"}); !errors.Is(err, mcpengine.ErrReadOnly) {
		t.Fatalf("read-only write: got %v, want ErrReadOnly", err)
	}
	if _, err := e.Delete(ctx, mcpengine.DeleteInput{ID: "x"}); !errors.Is(err, mcpengine.ErrReadOnly) {
		t.Fatalf("read-only delete: got %v, want ErrReadOnly", err)
	}
	if _, err := e.Restore(ctx, mcpengine.RestoreInput{ID: "x"}); !errors.Is(err, mcpengine.ErrReadOnly) {
		t.Fatalf("read-only restore: got %v, want ErrReadOnly", err)
	}
}

// TestEngine_RateLimited: a tenant over its write budget is rejected before the
// store is touched (so it needs no DB).
func TestEngine_RateLimited(t *testing.T) {
	// Burst 0 → the very first write is denied.
	e := mcpengine.NewEngine(nil, "tenant", mcpengine.WithWriteLimiter(mcpengine.NewWriteLimiter(5, 0)))
	ctx := context.Background()
	if _, err := e.Write(ctx, &mcpengine.WriteInput{ID: "x", Body: "body"}); !errors.Is(err, mcpengine.ErrRateLimited) {
		t.Fatalf("write: got %v, want ErrRateLimited", err)
	}
	if _, err := e.Delete(ctx, mcpengine.DeleteInput{ID: "x"}); !errors.Is(err, mcpengine.ErrRateLimited) {
		t.Fatalf("delete: got %v, want ErrRateLimited", err)
	}
}

func hasResult(rs []mcpengine.SearchHit, id string) bool {
	for i := range rs {
		if rs[i].ID == id {
			return true
		}
	}
	return false
}

func reMarshal(t *testing.T, v, dst any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}
