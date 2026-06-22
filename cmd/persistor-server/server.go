package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/persistorai/persistor/internal/index"
	"github.com/persistorai/persistor/internal/mcpauth"
	"github.com/persistorai/persistor/internal/mcpengine"
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
func newMux(getServer func(*http.Request) *mcp.Server, verifier auth.TokenVerifier, authOpts *auth.RequireBearerTokenOptions, metadata *protectedResourceMetadata, ready func(context.Context) error, protection *http.CrossOriginProtection) *http.ServeMux {
	// Persistor is a pure request/response tool server: no server-initiated
	// requests (sampling/elicitation/roots), no streaming results. Stateless +
	// JSONResponse is the right transport posture for that, and crucially for a
	// proxied/remote deployment:
	//   - JSONResponse returns a single application/json body per POST instead of
	//     a text/event-stream. SSE responses streamed through a reverse proxy
	//     (Tailscale Funnel today, an ALB on AWS later) are a known source of
	//     intermittent client failures; a single JSON body is proxy-friendly.
	//   - Stateless drops Mcp-Session-Id affinity, so a distributed client backend
	//     (claude.ai web) and future horizontal scaling don't depend on every
	//     request landing on the session's origin instance.
	// The tenant still comes only from the per-request bearer token (getServer
	// reads it from the request context), so isolation is unchanged.
	mcpHandler := mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	authed := auth.RequireBearerToken(verifier, authOpts)(mcpHandler)
	// Cross-origin protection (CSRF / DNS-rebinding): the SDK applies none with
	// nil options, and its localhost rebind guard doesn't cover the tailnet bind.
	// http.CrossOriginProtection keys off Sec-Fetch-Site, which only browsers
	// send, so non-browser MCP clients (Claude Code) are unaffected while a
	// browser cross-origin request is denied. Outermost so it rejects before auth.
	// The caller supplies it with its trusted browser origins (claude.ai web)
	// already registered via AddTrustedOrigin.
	mux := http.NewServeMux()
	// Cap the request body so one authenticated tenant can't exhaust memory/disk
	// with a single huge memory_write: the per-tenant write limiter caps
	// frequency, not size. 4 MiB comfortably fits a max-size note (the DB CHECK
	// bounds the body at 1 MiB) plus JSON-RPC framing.
	mux.Handle("/mcp", protection.Handler(limitBody(maxMCPBodyBytes, authed)))
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
// Writes are PG-native: Engine.Write goes straight to Postgres scoped to the
// token's tenant, so memory_write is fully tenant-isolated by construction — no
// shared write directory. Reads are tenant-isolated via RLS.
func tenantServer(store *index.Store, writeLimiter, readLimiter *mcpengine.KeyLimiter, version string) func(*http.Request) *mcp.Server {
	return func(r *http.Request) *mcp.Server {
		tenantID := ""
		readOnly := false
		surface := "remote-mcp"
		if ti := auth.TokenInfoFromContext(r.Context()); ti != nil {
			tenantID = ti.UserID
			readOnly = mcpauth.IsReadOnly(ti)
		}
		// Attribute the request to its tenant in the access log without the logger
		// having to parse the token itself.
		if st := reqStateFrom(r.Context()); st != nil {
			st.tenantID = tenantID
		}
		engine := mcpengine.NewEngine(store, tenantID,
			mcpengine.WithReadOnly(readOnly), mcpengine.WithSurface(surface),
			mcpengine.WithWriteLimiter(writeLimiter), mcpengine.WithReadLimiter(readLimiter))
		return mcpengine.NewServer(engine, version)
	}
}

// limitBody caps a handler's request body at maxBytes via http.MaxBytesReader, so
// a reader that exceeds it fails instead of buffering unboundedly into memory.
func limitBody(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

// perIPLimit rejects requests from a client IP over its rate, returning 429. Used
// to bound the open, unauthenticated /register proxy. The key is the connecting
// peer (RemoteAddr); X-Forwarded-For is deliberately ignored as it is spoofable.
func perIPLimit(limiter *mcpengine.KeyLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r)) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the connecting peer's IP (host part of RemoteAddr), falling back to
// the raw RemoteAddr if it has no port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
