package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
)

func TestMetricsRecordAndServe(t *testing.T) {
	m := newMetrics(func() dbpool.Stat { return dbpool.Stat{MaxConns: 8, IdleConns: 2} })
	m.record(http.StatusOK, 5)
	m.record(http.StatusNotFound, 2)
	m.record(http.StatusTooManyRequests, 1)
	m.record(http.StatusInternalServerError, 10)

	rr := httptest.NewRecorder()
	m.serveHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding metrics: %v", err)
	}
	want := map[string]float64{
		"requests_total":     4,
		"requests_2xx":       1,
		"requests_4xx":       2, // 404 + 429
		"requests_5xx":       1,
		"rate_limited_total": 1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if _, ok := got["db_pool"]; !ok {
		t.Error("db_pool block missing from metrics")
	}
}

func TestObserveSetsRequestIDAndCounts(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	m := newMetrics(nil)

	var sawState bool
	h := observe(log, m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawState = reqStateFrom(r.Context()) != nil
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody))

	if rr.Header().Get("X-Request-Id") == "" {
		t.Error("response is missing the X-Request-Id correlation header")
	}
	if !sawState {
		t.Error("reqState was not threaded into the request context")
	}
	if m.requestsTotal.Load() != 1 {
		t.Errorf("requests_total = %d, want 1", m.requestsTotal.Load())
	}
}

func TestObservePreservesIncomingRequestID(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	h := observe(log, newMetrics(nil), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Request-Id", "caller-supplied-id")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-Id"); got != "caller-supplied-id" {
		t.Errorf("X-Request-Id = %q, want the caller-supplied id echoed back", got)
	}
}

// The tenant allowlist must 403 valid-token holders outside it — with open
// provisioning, "any valid token" includes strangers, and /metrics leaks pool
// saturation. Empty allowlist keeps the any-valid-token behavior. TokenInfo is
// injected by the real RequireBearerToken middleware (the SDK context key is
// unexported), with a stub verifier that maps the bearer string to the tenant.
func TestRequireTenants(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{UserID: token, Expiration: time.Now().Add(time.Hour)}, nil
	}
	serve := func(allowed []string, tenant string) int {
		h := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{})(requireTenants(allowed, ok))
		req := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := serve(nil, "stranger"); got != http.StatusOK {
		t.Errorf("empty allowlist: status = %d, want 200 (any valid token admitted)", got)
	}
	if got := serve([]string{"operator-tenant"}, "operator-tenant"); got != http.StatusOK {
		t.Errorf("allowlisted tenant: status = %d, want 200", got)
	}
	if got := serve([]string{"operator-tenant"}, "stranger"); got != http.StatusForbidden {
		t.Errorf("other tenant: status = %d, want 403", got)
	}

	// Defense in depth: reached with no TokenInfo in context at all (mux
	// misassembly), a non-empty allowlist still refuses.
	rec := httptest.NewRecorder()
	requireTenants([]string{"operator-tenant"}, ok).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	if rec.Code != http.StatusForbidden {
		t.Errorf("no token info: status = %d, want 403", rec.Code)
	}
}
