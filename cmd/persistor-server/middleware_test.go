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

// TestCrossOriginProtection: a browser cross-origin POST to /mcp is rejected
// (before auth) by the cross-origin guard; a non-browser request (no
// Sec-Fetch-Site) is not blocked by it.
func TestCrossOriginProtection(t *testing.T) {
	verifier := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	}
	getServer := func(*http.Request) *mcp.Server { return nil }
	mux := newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, nil, nil)

	// Browser cross-origin request -> 403 from the cross-origin guard.
	cross := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	cross.Header.Set("Origin", "https://evil.example")
	crossRec := httptest.NewRecorder()
	mux.ServeHTTP(crossRec, cross)
	if crossRec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin /mcp status = %d, want 403", crossRec.Code)
	}

	// Non-browser request (no Sec-Fetch-Site) passes the guard and reaches auth,
	// which 401s the missing token — i.e. it was NOT blocked as cross-origin.
	plain := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	plainRec := httptest.NewRecorder()
	mux.ServeHTTP(plainRec, plain)
	if plainRec.Code != http.StatusUnauthorized {
		t.Fatalf("non-browser /mcp status = %d, want 401 (passed guard, hit auth)", plainRec.Code)
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
