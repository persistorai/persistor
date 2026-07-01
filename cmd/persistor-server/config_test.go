package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// setValidOIDCEnv sets the minimum environment loadConfig accepts.
func setValidOIDCEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://persistor_app:pw@db.example:25060/persistor?sslmode=require")
	t.Setenv("PERSISTOR_OIDC_ISSUER", "https://acme.customers.stytch.dev")
	t.Setenv("PERSISTOR_OIDC_AUDIENCE", "project-test-123")
	t.Setenv("PERSISTOR_OIDC_JWKS_URL", "https://acme.customers.stytch.dev/.well-known/jwks.json")
	// Neutralize optional knobs a developer shell might have set.
	for _, k := range []string{"PERSISTOR_LISTEN_ADDR", "PERSISTOR_PUBLIC_URL", "PERSISTOR_AUTO_MIGRATE",
		"PERSISTOR_LOG_LEVEL", "PERSISTOR_DB_MAX_CONNS", "PERSISTOR_TRUSTED_ORIGINS",
		"PERSISTOR_ORIGIN_SECRET", "PERSISTOR_DCR_ENABLED", "PERSISTOR_METRICS_TENANTS",
		"PERSISTOR_STYTCH_PUBLIC_TOKEN"} {
		t.Setenv(k, "")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	setValidOIDCEnv(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.listenAddr != defaultListenAddr {
		t.Errorf("listenAddr = %q, want default %q", cfg.listenAddr, defaultListenAddr)
	}
	if want := "http://" + defaultListenAddr; cfg.publicURL != want {
		t.Errorf("publicURL = %q, want derived %q", cfg.publicURL, want)
	}
	if !cfg.autoMigrate {
		t.Error("autoMigrate default should be true (self-host posture)")
	}
	if !cfg.dcrEnabled {
		t.Error("dcrEnabled default should be true (claude.ai onboarding)")
	}
	if cfg.originSecret != "" || cfg.metricsTenants != nil {
		t.Error("origin lock / metrics allowlist should default off")
	}
	if cfg.dbMaxConns != defaultDBMaxConns {
		t.Errorf("dbMaxConns = %d, want %d", cfg.dbMaxConns, defaultDBMaxConns)
	}
	if !reflect.DeepEqual(cfg.trustedOrigins, []string{defaultTrustedOrigin}) {
		t.Errorf("trustedOrigins = %v, want [%s]", cfg.trustedOrigins, defaultTrustedOrigin)
	}
	if cfg.publicHTTPS() {
		t.Error("publicHTTPS() = true for the derived http:// URL")
	}
}

func TestParseListAndTrustedOrigins(t *testing.T) {
	if got := parseList(" a , ,b "); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("parseList = %v", got)
	}
	if got := parseList("  "); got != nil {
		t.Errorf("parseList(blank) = %v, want nil", got)
	}
	if got := parseTrustedOrigins(""); !reflect.DeepEqual(got, []string{defaultTrustedOrigin}) {
		t.Errorf("parseTrustedOrigins(empty) = %v, want the claude.ai default", got)
	}
	if got := parseTrustedOrigins("https://a.example,https://b.example"); len(got) != 2 {
		t.Errorf("parseTrustedOrigins = %v", got)
	}
}

// TestBuildAuth exercises the real assembly of the OIDC verifier: JWKS fetched
// from a test server, metadata document pointing at the configured issuer, and
// a hard failure (not a silent no-auth fallback) when the JWKS is unreachable.
func TestBuildAuth(t *testing.T) {
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer jwks.Close()

	cfg := &serverConfig{
		publicURL:    "https://mcp.persistor.example",
		oidcIssuer:   "https://acme.customers.stytch.dev",
		oidcAudience: "project-test-123",
		oidcJWKSURL:  jwks.URL,
	}
	bundle, err := buildAuth(t.Context(), cfg, nil)
	if err != nil {
		t.Fatalf("buildAuth: %v", err)
	}
	if bundle.verify == nil {
		t.Fatal("nil verifier")
	}
	if want := cfg.publicURL + "/.well-known/oauth-protected-resource"; bundle.opts.ResourceMetadataURL != want {
		t.Errorf("ResourceMetadataURL = %q, want %q", bundle.opts.ResourceMetadataURL, want)
	}
	if bundle.metadata.Resource != cfg.publicURL {
		t.Errorf("metadata resource = %q, want %q", bundle.metadata.Resource, cfg.publicURL)
	}
	if len(bundle.metadata.AuthorizationServers) != 1 || bundle.metadata.AuthorizationServers[0] != cfg.oidcIssuer {
		t.Errorf("metadata AS = %v, want [%s]", bundle.metadata.AuthorizationServers, cfg.oidcIssuer)
	}

	// A token from the wrong issuer must fail verification end-to-end.
	if _, err := bundle.verify(t.Context(), "not-a-token", nil); err == nil {
		t.Error("verifier accepted garbage")
	}

	// Unreachable JWKS: keyfunc builds anyway (it logs and keeps retrying in
	// the background, so a JWKS blip at boot doesn't kill the daemon), and the
	// guarantee that matters is per-request fail-closed — with no keys, every
	// token is rejected, never accepted unverified.
	cfg.oidcJWKSURL = "http://127.0.0.1:1/jwks.json" // closed port
	bundle, err = buildAuth(t.Context(), cfg, nil)
	if err != nil {
		t.Fatalf("buildAuth with unreachable JWKS: %v (expected boot to survive)", err)
	}
	if _, err := bundle.verify(t.Context(), "eyJhbGciOiJSUzI1NiJ9.e30.sig", nil); err == nil {
		t.Error("verifier accepted a token with no JWKS available; must fail closed")
	}
}
