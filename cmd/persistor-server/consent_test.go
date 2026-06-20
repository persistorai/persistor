package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsentHandler(t *testing.T) {
	const token = "public-token-test-abc123"
	h, err := newConsentHandler(token)
	if err != nil {
		t.Fatalf("build consent handler: %v", err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	// The consent page carries OAuth query params and is iterated during setup —
	// it must never be served from cache.
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", cc)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	html := string(body)
	// The publishable token is injected, and the page renders Stytch's supported
	// React components (not the deprecated vanilla-js mount* helpers).
	for _, want := range []string{token, "StytchProvider", "StytchLogin", "IdentityProvider"} {
		if !strings.Contains(html, want) {
			t.Fatalf("consent page missing %q", want)
		}
	}
}
