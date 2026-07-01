package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/briancolinger/persistor/internal/dbpool"
)

// metrics holds dependency-free process counters for the /metrics endpoint: a
// long-running daemon otherwise has no per-call visibility. Counts are HTTP-level
// (every route), so they include tool calls (each MCP tool call is one POST to
// /mcp) without coupling the transport-agnostic engine to a metrics type. DB pool
// saturation is read live from the pool.
type metrics struct {
	requestsTotal atomic.Int64
	status2xx     atomic.Int64
	status4xx     atomic.Int64
	status5xx     atomic.Int64
	durationMs    atomic.Int64 // cumulative; mean = durationMs / requestsTotal
	rateLimited   atomic.Int64 // HTTP 429s (e.g. the /register per-IP limit)
	poolStat      func() dbpool.Stat
}

// newMetrics builds a metrics sink. poolStat may be nil (tests with no pool), in
// which case the db_pool block is omitted.
func newMetrics(poolStat func() dbpool.Stat) *metrics {
	return &metrics{poolStat: poolStat}
}

// record tallies one finished request by status class and duration.
func (m *metrics) record(status int, durationMs int64) {
	if m == nil {
		return
	}
	m.requestsTotal.Add(1)
	m.durationMs.Add(durationMs)
	switch {
	case status >= 500:
		m.status5xx.Add(1)
	case status == http.StatusTooManyRequests:
		m.rateLimited.Add(1)
		m.status4xx.Add(1)
	case status >= 400:
		m.status4xx.Add(1)
	default:
		m.status2xx.Add(1)
	}
}

// requireTenants restricts a bearer-gated handler to an allowlist of tenant ids
// (403 otherwise). With provisioning open, "any valid token" includes any
// self-provisioned stranger, and /metrics exposes global db_pool saturation —
// exactly the connection-exhaustion recon the bearer gate was meant to deny. An
// empty allowlist preserves the any-valid-token behavior (single-user
// self-host, where every token is the operator's).
func requireTenants(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ti := auth.TokenInfoFromContext(r.Context())
		if ti == nil || !slices.Contains(allowed, ti.UserID) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveHTTP writes the metrics snapshot as JSON: aggregate counters and DB pool
// saturation, no tenant-identifying data. It is bearer-gated at the mux (see
// buildHTTPServer) rather than public — on a public ingress, exposing pool
// capacity (max_conns / acquired_conns) would aid a connection-exhaustion
// attack, so a valid token is required to read it (plus the optional
// requireTenants operator allowlist).
func (m *metrics) serveHTTP(w http.ResponseWriter, _ *http.Request) {
	snap := map[string]any{
		"requests_total":      m.requestsTotal.Load(),
		"requests_2xx":        m.status2xx.Load(),
		"requests_4xx":        m.status4xx.Load(),
		"requests_5xx":        m.status5xx.Load(),
		"request_duration_ms": m.durationMs.Load(),
		"rate_limited_total":  m.rateLimited.Load(),
	}
	if m.poolStat != nil {
		snap["db_pool"] = m.poolStat()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		return
	}
}

// ctxKey is the unexported type for this package's context keys.
type ctxKey int

const reqStateKey ctxKey = iota

// reqState is per-request observability state threaded through the context: the
// correlation id (always set by the observe middleware) and the tenant (set by
// tenantServer once the bearer token resolves, so the access log can attribute
// the request without the logger needing to parse the token itself).
type reqState struct {
	requestID string
	tenantID  string
}

// reqStateFrom returns the request's observability state, or nil if absent.
func reqStateFrom(ctx context.Context) *reqState {
	st, ok := ctx.Value(reqStateKey).(*reqState)
	if !ok {
		return nil
	}
	return st
}
