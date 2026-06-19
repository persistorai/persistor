package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
	"github.com/briancolinger/persistor/internal/mcpengine"
)

// newSeededEngine connects to TEST_DATABASE_URL (skipping when unset), seeds a
// synthetic corpus, and returns an Engine. Mirrors the mcpengine test harness;
// the schema is assumed migrated (the loop gate runs fresh-migrate first).
func newSeededEngine(t *testing.T) *mcpengine.Engine {
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
	mustWrite(t, dir, "memory/daily/aurora.md", "# Aurora Protocol\n\nThe safety protocol for polar storms.\n")
	roots := []index.Root{{Name: "demo", Dir: dir}}

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store := index.NewStore(pool, log)
	indexer := index.NewIndexer(store, log, 0)
	if _, err := indexer.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("seed reindex: %v", err)
	}
	return mcpengine.NewEngine(store, indexer, tenantID, roots, filepath.Join(dir, "memory", "atomic"))
}

func mustWrite(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestHTTPTransport_ToolsOverStreamableHTTP proves the daemon serves the MCP
// tool surface over real HTTP: a go-sdk client connects to /mcp, lists the four
// tools, and calls memory_search end to end. This is the automated half of the
// P1 gate (the cross-device tailnet check is manual).
func TestHTTPTransport_ToolsOverStreamableHTTP(t *testing.T) {
	engine := newSeededEngine(t)
	ctx := context.Background()

	server := mcpengine.NewServer(engine, "test")
	ts := httptest.NewServer(newMux(server))
	defer ts.Close()

	// Liveness probe.
	healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/healthz", http.NoBody)
	if err != nil {
		t.Fatalf("healthz request: %v", err)
	}
	resp, err := http.DefaultClient.Do(healthReq)
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}

	// Connect a real MCP client over the Streamable HTTP transport.
	transport := &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if got := len(tools.Tools); got != 4 {
		t.Fatalf("listed %d tools, want 4", got)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": "polar storm protocol"},
	})
	if err != nil {
		t.Fatalf("call memory_search: %v", err)
	}
	if res.IsError {
		t.Fatalf("memory_search returned error: %+v", res.Content)
	}

	var out mcpengine.SearchOutput
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	found := false
	for _, r := range out.Results {
		if r.ID == "demo:memory-daily-aurora" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory_search over HTTP missed aurora: %+v", out.Results)
	}
}
