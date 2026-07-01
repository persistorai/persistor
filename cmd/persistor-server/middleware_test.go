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
	h := securityHeaders(false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	// HSTS is gated off for a non-HTTPS (tailnet) deployment.
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS set with hsts=false: %q", got)
	}
}

// TestSecurityHeadersHSTS verifies HSTS is set when the deployment is public
// HTTPS (hsts=true).
func TestSecurityHeadersHSTS(t *testing.T) {
	h := securityHeaders(true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody))
	if got := rec.Header().Get("Strict-Transport-Security"); got != hstsValue {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, hstsValue)
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
	protection := http.NewCrossOriginProtection()
	if err := protection.AddTrustedOrigin("https://claude.ai"); err != nil {
		t.Fatalf("AddTrustedOrigin: %v", err)
	}
	mux := newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, nil, nil, protection, false)

	// Untrusted browser cross-origin request -> 403 from the cross-origin guard.
	cross := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	cross.Header.Set("Origin", "https://evil.example")
	crossRec := httptest.NewRecorder()
	mux.ServeHTTP(crossRec, cross)
	if crossRec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin /mcp status = %d, want 403", crossRec.Code)
	}

	// A TRUSTED browser origin (claude.ai) passes the guard and reaches auth,
	// which 401s the missing token — i.e. it was NOT blocked as cross-origin.
	trusted := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	trusted.Header.Set("Sec-Fetch-Site", "cross-site")
	trusted.Header.Set("Origin", "https://claude.ai")
	trustedRec := httptest.NewRecorder()
	mux.ServeHTTP(trustedRec, trusted)
	if trustedRec.Code != http.StatusUnauthorized {
		t.Fatalf("trusted-origin /mcp status = %d, want 401 (passed guard, hit auth)", trustedRec.Code)
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
			mux := newMux(getServer, verifier, &auth.RequireBearerTokenOptions{}, nil, tt.ready, http.NewCrossOriginProtection(), false)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
			if rec.Code != tt.want {
				t.Fatalf("/readyz status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// TestCORSPreflight verifies an OPTIONS preflight is answered with 204 and the
// CORS headers a browser MCP client needs — without reaching the next handler.
func TestCORSPreflight(t *testing.T) {
	called := false
	h := cors([]string{"https://claude.ai"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodOptions, "/mcp", http.NoBody)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if called {
		t.Error("preflight must not reach the next handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://claude.ai" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "authorization,content-type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want the requested headers echoed", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods missing")
	}
}

// TestCORSActualRequest verifies a non-preflight request gets CORS headers and
// still reaches the wrapped handler.
func TestCORSActualRequest(t *testing.T) {
	called := false
	h := cors([]string{"https://claude.ai"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Origin", "https://claude.ai")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("non-preflight request must reach the next handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://claude.ai" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != corsExposeHeaders {
		t.Errorf("Access-Control-Expose-Headers = %q, want %q", got, corsExposeHeaders)
	}
}

// TestCORSUntrustedOrigin verifies an origin outside the trusted list gets no
// CORS allow headers: the preflight is answered 204 bare (a browser denial) and
// an actual request passes through without Access-Control-Allow-Origin.
func TestCORSUntrustedOrigin(t *testing.T) {
	t.Run("preflight gets no allow headers", func(t *testing.T) {
		called := false
		h := cors([]string{"https://claude.ai"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		req := httptest.NewRequest(http.MethodOptions, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 204", rec.Code)
		}
		if called {
			t.Error("preflight must not reach the next handler")
		}
		for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
			if got := rec.Header().Get(header); got != "" {
				t.Errorf("%s = %q, want unset for an untrusted origin", header, got)
			}
		}
	})
	t.Run("actual request passes through without allow-origin", func(t *testing.T) {
		h := cors([]string{"https://claude.ai"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want unset for an untrusted origin", got)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 (auth remains the access boundary)", rec.Code)
		}
	})
}
