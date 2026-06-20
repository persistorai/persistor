package main

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpengine"
)

// newMux builds the HTTP routing for the remote MCP daemon: the auth-gated
// Streamable HTTP transport at /mcp, and an open liveness probe at /healthz.
// Identity comes only from the verified bearer token (Non-corner-painting rule
// 1: never from the network path); the bearer middleware sits in front of the
// MCP handler and 401s missing/invalid tokens with a WWW-Authenticate header.
func newMux(getServer func(*http.Request) *mcp.Server, verifier auth.TokenVerifier, authOpts *auth.RequireBearerTokenOptions) *http.ServeMux {
	mcpHandler := mcp.NewStreamableHTTPHandler(getServer, nil)
	authed := auth.RequireBearerToken(verifier, authOpts)(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("/mcp", authed)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
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
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil {
			tenantID = ti.UserID
		}
		engine := mcpengine.NewEngine(store, indexer, tenantID, roots, writeDir)
		return mcpengine.NewServer(engine, version)
	}
}
