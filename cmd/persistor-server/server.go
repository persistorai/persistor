package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/briancolinger/persistor/internal/index"
	"github.com/briancolinger/persistor/internal/mcpauth"
	"github.com/briancolinger/persistor/internal/mcpengine"
)

// protectedResourceMetadata is the RFC 9728 OAuth Protected Resource Metadata
// document. An MCP client fetches it (the URL is advertised in the 401
// WWW-Authenticate header) to discover which Authorization Server issues tokens
// for this resource.
type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

func (m *protectedResourceMetadata) serveHTTP(w http.ResponseWriter, _ *http.Request) {
	body, err := json.Marshal(m)
	if err != nil {
		http.Error(w, "metadata encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		return
	}
}

// newMux builds the HTTP routing for the remote MCP daemon: the auth-gated
// Streamable HTTP transport at /mcp, an open liveness probe at /healthz, and —
// in OIDC mode — the open protected-resource metadata document. Identity comes
// only from the verified bearer token (Non-corner-painting rule 1: never from
// the network path); the bearer middleware sits in front of the MCP handler and
// 401s missing/invalid tokens with a WWW-Authenticate header.
// ready probes a dependency (the DB pool) for the /readyz handler; nil means the
// daemon reports ready unconditionally (used in tests with no pool).
func newMux(getServer func(*http.Request) *mcp.Server, verifier auth.TokenVerifier, authOpts *auth.RequireBearerTokenOptions, metadata *protectedResourceMetadata, ready func(context.Context) error) *http.ServeMux {
	mcpHandler := mcp.NewStreamableHTTPHandler(getServer, nil)
	authed := auth.RequireBearerToken(verifier, authOpts)(mcpHandler)
	// Cross-origin protection (CSRF / DNS-rebinding): the SDK applies none with
	// nil options, and its localhost rebind guard doesn't cover the tailnet bind.
	// http.CrossOriginProtection keys off Sec-Fetch-Site, which only browsers
	// send, so non-browser MCP clients (Claude Code) are unaffected while a
	// browser cross-origin request is denied. Outermost so it rejects before auth.
	// A future browser client (claude.ai) is added via AddTrustedOrigin.
	protection := http.NewCrossOriginProtection()
	mux := http.NewServeMux()
	mux.Handle("/mcp", protection.Handler(authed))
	// /healthz is pure liveness (the process is up); /readyz also checks the DB,
	// the daemon's only hard dependency, so an orchestrator won't route to an
	// instance whose Postgres is unreachable.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	if metadata != nil {
		mux.HandleFunc("/.well-known/oauth-protected-resource", metadata.serveHTTP)
	}
	return mux
}

// tenantServer returns the getServer function the Streamable HTTP handler calls
// per session. It binds each session to the tenant carried by its verified
// token: the bearer middleware runs first and stores a TokenInfo whose UserID is
// the tenant, so tool calls set app.tenant_id from the token rather than daemon
// config. The SDK also pins the session to that UserID (anti-hijack).
//
// All tenants share the configured file-backed roots/writeDir, so memory_write
// is effectively single-tenant for now; true multi-tenant writes move to the
// PG-native path in a later phase. Read tools are fully tenant-isolated via RLS.
func tenantServer(store *index.Store, indexer *index.Indexer, roots []index.Root, writeDir, version string) func(*http.Request) *mcp.Server {
	return func(r *http.Request) *mcp.Server {
		tenantID := ""
		readOnly := false
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil {
			tenantID = ti.UserID
			readOnly = mcpauth.IsReadOnly(ti)
		}
		engine := mcpengine.NewEngine(store, indexer, tenantID, roots, writeDir, mcpengine.WithReadOnly(readOnly))
		return mcpengine.NewServer(engine, version)
	}
}
