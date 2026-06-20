package mcpengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpengine"
)

// newTestEngine connects to TEST_DATABASE_URL (skipping when unset), seeds a
// synthetic corpus, and returns an Engine plus the notes dir. The schema is
// assumed migrated (the loop gate runs the fresh-migration step first).
func newTestEngine(t *testing.T) *mcpengine.Engine {
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
	t.Cleanup(func() {
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
		_, _ = tx.Exec(clean, "DELETE FROM notes WHERE tenant_id = current_setting('app.tenant_id')::uuid")
		_, _ = tx.Exec(clean, "DELETE FROM sources WHERE tenant_id = current_setting('app.tenant_id')::uuid")
		_ = tx.Commit(clean)
	})

	dir := t.TempDir()
	writeFile(t, dir, "identity/SOUL.md", "# Soul\n\nWe are the Northwind expedition crew.\n")
	writeFile(t, dir, "memory/daily/aurora.md", "# Aurora Protocol\n\nThe safety protocol for polar storms.\n")
	roots := []index.Root{{Name: "scout", Dir: dir, CorePaths: []string{"identity/SOUL.md"}}}

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store := index.NewStore(pool, log)
	indexer := index.NewIndexer(store, log, 0)
	if _, err := indexer.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("seed reindex: %v", err)
	}

	notesDir := filepath.Join(dir, "memory", "atomic")
	return mcpengine.NewEngine(store, indexer, tenantID, roots, notesDir)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestEngine_RoundTrip exercises all four tool backends against the test DB.
func TestEngine_RoundTrip(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// search: finds the Aurora note.
	sr, err := e.Search(ctx, mcpengine.SearchInput{Query: "polar storm protocol"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !hasResult(sr.Results, "scout:memory-daily-aurora") {
		t.Fatalf("search missed aurora: %+v", sr.Results)
	}

	// get: returns the full body.
	gr, err := e.Get(ctx, mcpengine.GetInput{ID: "scout:memory-daily-aurora"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !gr.Found || gr.Body == "" {
		t.Fatalf("get found=%v body=%q", gr.Found, gr.Body)
	}

	// write: add a new note that supersedes the Aurora protocol.
	wr, err := e.Write(ctx, &mcpengine.WriteInput{
		Path: "aurora-v2.md", ID: "scout:aurora-v2", Supersedes: "scout:memory-daily-aurora",
		Body: "# Aurora Protocol v2\n\nUpdated polar storm protocol.",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if wr.Written != "aurora-v2.md" || wr.Superseded != 1 {
		t.Fatalf("write = %+v, want aurora-v2.md superseded=1", wr)
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

// TestMCPRoundTrip drives the server over an in-memory transport with a real MCP
// client, proving the wire protocol + schema inference work end to end through
// the shared NewServer constructor.
func TestMCPRoundTrip(t *testing.T) {
	e := newTestEngine(t)
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
// same note it lands on is rejected,
// while superseding a different note still works.
func TestEngine_WriteRejectsSelfSupersede(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// The note at notes/widget.md resolves to id "scout:widget" (the test engine's
	// notesDir is <dir>/memory/atomic under root "scout"; derive accordingly).
	first, err := e.Write(ctx, &mcpengine.WriteInput{Path: "widget.md", Body: "# Widget\n\nOriginal."})
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	selfID := "scout:" + "memory-atomic-widget"

	// Self-supersede (same path → same id) must be rejected, not a silent no-op.
	_, err = e.Write(ctx, &mcpengine.WriteInput{Path: "widget.md", Supersedes: selfID, Body: "# Widget\n\nEdit."})
	if err == nil {
		t.Errorf("self-supersede write should error, got nil (first wrote %q)", first.Written)
	}

	// Superseding a DIFFERENT note (new path) is allowed and forks history.
	w, err := e.Write(ctx, &mcpengine.WriteInput{Path: "widget-v2.md", Supersedes: selfID, Body: "# Widget v2\n\nNew."})
	if err != nil {
		t.Fatalf("legitimate supersede write: %v", err)
	}
	if w.Superseded != 1 {
		t.Errorf("legitimate supersede: Superseded = %d, want 1", w.Superseded)
	}
}

func TestEngine_WriteRejectsMissingSupersedeTarget(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// Superseding an id that does not exist must be rejected, not a silent write
	// with a dangling pointer (a prompt-injection memory-poisoning vector).
	_, err := e.Write(ctx, &mcpengine.WriteInput{
		Path: "ghost.md", Supersedes: "scout:does-not-exist", Body: "# Ghost\n\nbody",
	})
	if err == nil {
		t.Fatal("supersede of a non-existent note should error, got nil")
	}
}

// TestEngine_ReadOnlyRejectsWrite verifies a read-only engine denies memory_write
// before touching the store (so it needs no DB).
func TestEngine_ReadOnlyRejectsWrite(t *testing.T) {
	e := mcpengine.NewEngine(nil, nil, "tenant", nil, t.TempDir(), mcpengine.WithReadOnly(true))
	_, err := e.Write(context.Background(), &mcpengine.WriteInput{Path: "x.md", Body: "# X\n\nbody"})
	if !errors.Is(err, mcpengine.ErrReadOnly) {
		t.Fatalf("read-only write: got %v, want ErrReadOnly", err)
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
