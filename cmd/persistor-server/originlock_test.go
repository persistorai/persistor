package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/persistorai/persistor/internal/mcpengine"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestOriginLockDisabledPassesThrough(t *testing.T) {
	h := originLock("", okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOriginLockEnforced(t *testing.T) {
	h := originLock("edge-secret", okHandler())

	tests := []struct {
		name   string
		path   string
		header string
		want   int
	}{
		{"missing header rejected", "/mcp", "", http.StatusForbidden},
		{"wrong header rejected", "/mcp", "not-the-secret", http.StatusForbidden},
		{"matching header passes", "/mcp", "edge-secret", http.StatusOK},
		{"metadata locked too", "/.well-known/oauth-protected-resource", "", http.StatusForbidden},
		{"healthz exempt for the platform liveness probe", "/healthz", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			if tt.header != "" {
				req.Header.Set(originSecretHeader, tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

// The per-IP limiter must key on CF-Connecting-IP only when the origin lock
// vouches for it (trustCF); otherwise the spoofable header is ignored and all
// traffic from one peer shares a bucket.
func TestPerIPLimitClientKey(t *testing.T) {
	t.Run("trustCF keys on CF-Connecting-IP", func(t *testing.T) {
		limiter := mcpengine.NewKeyLimiter(1, 1) // 1 rps, burst 1: second hit per key is limited
		h := perIPLimit(limiter, true, okHandler())
		for i, ip := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
			req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
			req.RemoteAddr = "203.0.113.10:443" // same proxy peer for all
			req.Header.Set("CF-Connecting-IP", ip)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "distinct client %d must get its own bucket", i)
		}
		// Same client again exhausts its own bucket.
		req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
		req.RemoteAddr = "203.0.113.10:443"
		req.Header.Set("CF-Connecting-IP", "198.51.100.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("without trustCF the header is ignored", func(t *testing.T) {
		limiter := mcpengine.NewKeyLimiter(1, 1)
		h := perIPLimit(limiter, false, okHandler())
		first := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
		first.RemoteAddr = "203.0.113.10:443"
		first.Header.Set("CF-Connecting-IP", "198.51.100.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, first)
		assert.Equal(t, http.StatusOK, rec.Code)

		// A "different" spoofed client IP from the same peer must NOT earn a
		// fresh bucket.
		second := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
		second.RemoteAddr = "203.0.113.10:443"
		second.Header.Set("CF-Connecting-IP", "198.51.100.2")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, second)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})
}
