package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testPublicURL     = "https://devbox.example.ts.net"
	testStytchIssuer  = "https://acme.customers.stytch.dev"
	testFacadeJWKSURL = "https://acme.customers.stytch.dev/.well-known/jwks.json"
)

func TestNewAuthServerMetadata(t *testing.T) {
	m := newAuthServerMetadata(testPublicURL, testStytchIssuer, testFacadeJWKSURL)

	// RFC 8414: the issuer MUST equal the base the doc is fetched from (this
	// server), and the authorization endpoint is this server's consent page.
	if m.Issuer != testPublicURL {
		t.Errorf("issuer = %q, want %q", m.Issuer, testPublicURL)
	}
	if want := testPublicURL + "/authorize"; m.AuthorizationEndpoint != want {
		t.Errorf("authorization_endpoint = %q, want %q", m.AuthorizationEndpoint, want)
	}
	// Registration stays same-origin so clients that insist on it still work.
	if want := testPublicURL + "/register"; m.RegistrationEndpoint != want {
		t.Errorf("registration_endpoint = %q, want %q", m.RegistrationEndpoint, want)
	}
	// Token/userinfo/jwks delegate to the upstream IdP.
	if want := testStytchIssuer + "/v1/oauth2/token"; m.TokenEndpoint != want {
		t.Errorf("token_endpoint = %q, want %q", m.TokenEndpoint, want)
	}
	if want := testStytchIssuer + "/v1/oauth2/userinfo"; m.UserinfoEndpoint != want {
		t.Errorf("userinfo_endpoint = %q, want %q", m.UserinfoEndpoint, want)
	}
	if m.JWKSURI != testFacadeJWKSURL {
		t.Errorf("jwks_uri = %q, want %q", m.JWKSURI, testFacadeJWKSURL)
	}
	if len(m.CodeChallengeMethodsSupported) != 1 || m.CodeChallengeMethodsSupported[0] != "S256" {
		t.Errorf("code_challenge_methods_supported = %v, want [S256]", m.CodeChallengeMethodsSupported)
	}
}

func TestStytchEndpointTrimsSlash(t *testing.T) {
	if got := stytchEndpoint(testStytchIssuer+"/", "/v1/oauth2/token"); got != testStytchIssuer+"/v1/oauth2/token" {
		t.Errorf("stytchEndpoint did not trim trailing slash: %q", got)
	}
}

func TestAuthServerMetadataServeHTTP(t *testing.T) {
	m := newAuthServerMetadata(testPublicURL, testStytchIssuer, testFacadeJWKSURL)
	rec := httptest.NewRecorder()
	m.serveHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var got authServerMetadata
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got.Issuer != testPublicURL {
		t.Errorf("decoded issuer = %q, want %q", got.Issuer, testPublicURL)
	}
}

func TestRegisterProxyRelaysPOST(t *testing.T) {
	const wantClientID = "connected-app-test-123"
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("upstream got method %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"client_id":"`+wantClientID+`"}`)
	}))
	defer upstream.Close()

	proxy := newRegisterProxy(upstream.Client(), upstream.URL)
	rec := httptest.NewRecorder()
	reqBody := `{"redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`
	proxy(rec, httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(reqBody)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if gotBody != reqBody {
		t.Errorf("upstream received body %q, want %q", gotBody, reqBody)
	}
	if !strings.Contains(rec.Body.String(), wantClientID) {
		t.Errorf("response %q does not contain client_id %q", rec.Body.String(), wantClientID)
	}
}

func TestRegisterProxyRejectsNonPOST(t *testing.T) {
	proxy := newRegisterProxy(http.DefaultClient, "https://unused.example/register")
	rec := httptest.NewRecorder()
	proxy(rec, httptest.NewRequest(http.MethodGet, "/register", http.NoBody))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow = %q, want POST", allow)
	}
}
