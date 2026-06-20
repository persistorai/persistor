package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSecurityHeaders verifies the hardening headers (clickjacking + sniffing)
// are set on every response.
func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/authorize", http.NoBody))

	want := map[string]string{
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "frame-ancestors 'none'",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
}

// TestReadyz checks the readiness probe reflects the dependency probe's result.
func TestReadyz(t *testing.T) {
	verifier := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	}
	getServer := func(*http.Request) *mcp.Server { return nil }

	tests := []struct {
		name  string
		ready func(context.Context) error
		want  int
	}{
		{name: "nil probe is ready", ready: nil, want: http.StatusOK},
		{name: "healthy probe", ready: func(context.Context) error { return nil }, want: http.StatusOK},
		{name: "failing probe", ready: func(context.Context) error { return errors.New("db down") }, want: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, nil, tt.ready)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
			if rec.Code != tt.want {
				t.Fatalf("/readyz status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
