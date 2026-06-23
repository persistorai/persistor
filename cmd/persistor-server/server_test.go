package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
	"github.com/briancolinger/persistor/internal/mcpauth"
)

const (
	testIssuer   = "https://test.stytch.example"
	testAudience = "persistor-test"
)

// testStack is an in-process daemon: the real OIDC-gated mux over a test DB. It
// mints its own RS256 tokens against an in-memory key, so no IdP is needed.
type testStack struct {
	ts      *httptest.Server
	store   *index.Store
	pool    *dbpool.Pool
	signKey *rsa.PrivateKey
}

// newTestStack stands up the auth-gated HTTP handler against TEST_DATABASE_URL
// (skipping when unset). The schema is assumed migrated (the loop gate runs
// fresh-migrate first). The verifier derives the tenant straight from iss|sub
// (no resolver), so a token's subject determines its tenant deterministically.
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

	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	keyFunc := func(*jwt.Token) (any, error) { return &signKey.PublicKey, nil }

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store := index.NewStore(pool, log)

	verifier := mcpauth.NewOIDCAuth(keyFunc, testIssuer, testAudience)
	getServer := tenantServer(store, nil, nil, "test", log)
	authOpts := &auth.RequireBearerTokenOptions{ResourceMetadataURL: "http://example/.well-known/oauth-protected-resource"}
	ts := httptest.NewServer(newMux(getServer, verifier.Verify, authOpts, nil, pool.Ping, http.NewCrossOriginProtection()))
	t.Cleanup(ts.Close)

	return &testStack{ts: ts, store: store, pool: pool, signKey: signKey}
}

// seededNoteID is the id every seeded note lands at; isolation comes from the
// tenant, not the id.
const seededNoteID = "demo:memory-daily-note"

// tenant seeds a fresh tenant with one note and returns a signed bearer token for
// it. Each call uses a unique OIDC subject so the derived tenant (uuidv5(iss|sub))
// is unique per run — repeatable regardless of prior runs. The verifier derives
// the same tenant from the token, so the seeded note and the request land together.
func (s *testStack) tenant(t *testing.T, noteBody string) string {
	t.Helper()
	ctx := context.Background()
	subject := "test-subject-" + uuid.NewString()
	tenantID := mcpauth.TenantForSubject(testIssuer, subject)

	if _, err := s.store.WriteNote(ctx, tenantID, &index.PGNoteInput{
		ID: seededNoteID, Title: "Note", Body: noteBody, Surface: "test",
	}, 0); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	t.Cleanup(func() {
		clean := context.Background()
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
	return s.token(t, subject)
}

// token mints an RS256 bearer token for the subject, signed with the stack's
// in-memory key.
func (s *testStack) token(t *testing.T, subject string) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{testAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(s.signKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
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
	req2.Header.Set("Authorization", "Bearer not.a.jwt")
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

// TestHTTP_TenantIsolation: two subjects (two tenants), two tokens; each token's
// session sees only its own tenant's notes through the full HTTP + auth + RLS path.
func TestHTTP_TenantIsolation(t *testing.T) {
	st := newTestStack(t)
	tokenA := st.tenant(t, "# Note\n\nalpha secret protocol\n")
	tokenB := st.tenant(t, "# Note\n\nbravo secret protocol\n")

	// Tenant A's token: lists tools, sees alpha, never bravo.
	if n := listToolCount(t, st.ts.URL, tokenA); n != 8 {
		t.Fatalf("tenant A listed %d tools, want 8", n)
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
	ts := httptest.NewServer(newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, meta, nil, http.NewCrossOriginProtection()))
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

// TestBuildHTTPServer_MetricsGatedAndHSTS exercises the real server wiring: on a
// public HTTPS deployment /metrics requires a bearer token (no longer public)
// and responses carry HSTS. No DB needed — the verifier rejects every token and
// the routes under test don't touch the store.
func TestBuildHTTPServer_MetricsGatedAndHSTS(t *testing.T) {
	cfg := serverConfig{
		listenAddr:     "127.0.0.1:0",
		publicURL:      "https://mcp.test.example",
		trustedOrigins: []string{"https://claude.ai"},
	}
	authn := authBundle{
		verify: func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
			return nil, auth.ErrInvalidToken
		},
		opts: &auth.RequireBearerTokenOptions{ResourceMetadataURL: cfg.publicURL + "/.well-known/oauth-protected-resource"},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	srv, err := buildHTTPServer(&cfg, nil, authn, log, nil, func() dbpool.Stat { return dbpool.Stat{} })
	if err != nil {
		t.Fatalf("buildHTTPServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// /metrics without a token -> 401 (was unauthenticated before the fix).
	mreq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/metrics", http.NoBody)
	mresp, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	_ = mresp.Body.Close()
	if mresp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/metrics without token = %d, want 401", mresp.StatusCode)
	}

	// HSTS advertised for the public HTTPS deployment; /healthz stays open.
	hreq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/healthz", http.NoBody)
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	_ = hresp.Body.Close()
	if hresp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %d, want 200", hresp.StatusCode)
	}
	if got := hresp.Header.Get("Strict-Transport-Security"); got != hstsValue {
		t.Fatalf("HSTS = %q, want %q", got, hstsValue)
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
	// Persistor returns results as a JSON text block, not structuredContent, for
	// broad client compatibility (see mcpengine.registerTools).
	var out mcpengineSearchOutput
	if len(res.Content) == 0 {
		t.Fatalf("search %q: result has no content", query)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("search %q: content[0] is %T, want *mcp.TextContent", query, res.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make([]string, len(out.Results))
	for i, r := range out.Results {
		ids[i] = r.ID
	}
	return ids
}

// mcpengineSearchOutput mirrors the memory_search result shape for decoding.
type mcpengineSearchOutput struct {
	Results []struct {
		ID string `json:"id"`
	} `json:"results"`
}

func hasSeededNote(ids []string) bool {
	for _, id := range ids {
		if id == seededNoteID {
			return true
		}
	}
	return false
}
