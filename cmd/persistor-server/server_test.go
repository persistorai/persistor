package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
	"github.com/briancolinger/persistor/internal/mcpauth"
	"github.com/briancolinger/persistor/internal/mcpengine"
)

// testStack is an in-process daemon: the real auth-gated mux over a test DB.
type testStack struct {
	ts       *httptest.Server
	store    *index.Store
	keyStore *mcpauth.PGKeyStore
	pool     *dbpool.Pool
}

// newTestStack stands up the auth-gated HTTP handler against TEST_DATABASE_URL
// (skipping when unset). The schema is assumed migrated (the loop gate runs
// fresh-migrate first).
func newTestStack(t *testing.T) *testStack {
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

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store := index.NewStore(pool, log)
	keyStore := mcpauth.NewPGKeyStore(pool)

	verifier := mcpauth.NewStaticTokenAuth(keyStore)
	getServer := tenantServer(store, "test")
	authOpts := &auth.RequireBearerTokenOptions{ResourceMetadataURL: "http://example/.well-known/oauth-protected-resource"}
	ts := httptest.NewServer(newMux(getServer, verifier.Verify, authOpts, nil, pool.Ping))
	t.Cleanup(ts.Close)

	return &testStack{ts: ts, store: store, keyStore: keyStore, pool: pool}
}

// seededNoteID is the id every seeded note lands at; isolation comes from the
// tenant, not the path.
const seededNoteID = "demo:memory-daily-note"

// tenant seeds one tenant with a single note and mints an API key for it,
// returning the raw token.
func (s *testStack) tenant(t *testing.T, noteBody string) string {
	t.Helper()
	ctx := context.Background()
	tenantID := uuid.New().String()

	if _, err := s.store.WriteNote(ctx, tenantID, &index.PGNoteInput{
		ID: seededNoteID, Title: "Note", Body: noteBody, Surface: "test",
	}, 0); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	token, err := s.keyStore.CreateAPIKey(ctx, tenantID, "test")
	if err != nil {
		t.Fatalf("mint key: %v", err)
	}
	t.Cleanup(func() {
		clean := context.Background()
		_, _ = s.pool.Exec(clean, "DELETE FROM api_keys WHERE tenant_id = $1", tenantID)
		tx, err := s.pool.Begin(clean)
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
	})
	return token
}

// TestHTTP_AuthRequired: the /mcp endpoint rejects missing/invalid tokens with a
// 401 and a WWW-Authenticate header; /healthz stays open.
func TestHTTP_AuthRequired(t *testing.T) {
	st := newTestStack(t)
	ctx := context.Background()

	// No token -> 401 + WWW-Authenticate.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, st.ts.URL+"/mcp", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatal("401 missing WWW-Authenticate header")
	}

	// Bogus token -> 401.
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, st.ts.URL+"/mcp", strings.NewReader("{}"))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer psk_bogus")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("post bogus: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus-token status = %d, want 401", resp2.StatusCode)
	}

	// healthz stays open.
	hreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, st.ts.URL+"/healthz", http.NoBody)
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	_ = hresp.Body.Close()
	if hresp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", hresp.StatusCode)
	}
}

// TestHTTP_TenantIsolation: two tenants, two tokens; each token's session sees
// only its own tenant's notes through the full HTTP + auth + RLS path.
func TestHTTP_TenantIsolation(t *testing.T) {
	st := newTestStack(t)
	tokenA := st.tenant(t, "# Note\n\nalpha secret protocol\n")
	tokenB := st.tenant(t, "# Note\n\nbravo secret protocol\n")

	// Tenant A's token: lists tools, sees alpha, never bravo.
	if n := listToolCount(t, st.ts.URL, tokenA); n != 6 {
		t.Fatalf("tenant A listed %d tools, want 6", n)
	}
	if !hasSeededNote(searchOverHTTP(t, st.ts.URL, tokenA, "alpha")) {
		t.Fatal("tenant A could not find its own note")
	}
	if hasSeededNote(searchOverHTTP(t, st.ts.URL, tokenA, "bravo")) {
		t.Fatal("tenant A leaked tenant B's note")
	}

	// Tenant B's token: sees bravo, never alpha.
	if !hasSeededNote(searchOverHTTP(t, st.ts.URL, tokenB, "bravo")) {
		t.Fatal("tenant B could not find its own note")
	}
	if hasSeededNote(searchOverHTTP(t, st.ts.URL, tokenB, "alpha")) {
		t.Fatal("tenant B leaked tenant A's note")
	}
}

// TestProtectedResourceMetadata: in OIDC mode the daemon serves the RFC 9728
// document (open, no auth) advertising the Authorization Server.
func TestProtectedResourceMetadata(t *testing.T) {
	meta := &protectedResourceMetadata{
		Resource:             "https://persistor.example",
		AuthorizationServers: []string{"https://issuer.example"},
	}
	verifier := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	}
	getServer := func(*http.Request) *mcp.Server { return nil }
	ts := httptest.NewServer(newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, meta, nil))
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		ts.URL+"/.well-known/oauth-protected-resource", http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200", resp.StatusCode)
	}
	var got protectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Resource != meta.Resource || len(got.AuthorizationServers) != 1 ||
		got.AuthorizationServers[0] != meta.AuthorizationServers[0] {
		t.Fatalf("metadata = %+v, want %+v", got, meta)
	}
}

// bearerClient is an http.Client that attaches a static bearer token to every
// request (POST and the standalone SSE GET).
func bearerClient(token string) *http.Client {
	return &http.Client{Transport: bearerRoundTripper{token: token, base: http.DefaultTransport}}
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func connectClient(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: bearerClient(token)}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func listToolCount(t *testing.T, url, token string) int {
	t.Helper()
	cs := connectClient(t, url, token)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	return len(tools.Tools)
}

func searchOverHTTP(t *testing.T, url, token, query string) []string {
	t.Helper()
	cs := connectClient(t, url, token)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": query},
	})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if res.IsError {
		t.Fatalf("search %q returned error: %+v", query, res.Content)
	}
	var out mcpengine.SearchOutput
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make([]string, len(out.Results))
	for i, r := range out.Results {
		ids[i] = r.ID
	}
	return ids
}

func hasSeededNote(ids []string) bool {
	for _, id := range ids {
		if id == seededNoteID {
			return true
		}
	}
	return false
}
