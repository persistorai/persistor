package main

import "testing"

// TestRequireHTTPS_ExactHost verifies the loopback carve-out matches the host
// exactly, so a look-alike like http://127.0.0.1.attacker.tld is rejected.
func TestRequireHTTPS_ExactHost(t *testing.T) {
	ok := []string{
		"https://issuer.example",
		"http://127.0.0.1:8200/jwks",
		"http://localhost:9000",
		"http://[::1]:8200",
	}
	for _, u := range ok {
		if err := requireHTTPS("X", u); err != nil {
			t.Errorf("requireHTTPS(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{
		"http://insecure.example",
		"http://127.0.0.1.attacker.tld/jwks",
		"http://localhost.attacker.tld",
		"http://evil.com",
	}
	for _, u := range bad {
		if err := requireHTTPS("X", u); err == nil {
			t.Errorf("requireHTTPS(%q) = nil, want error", u)
		}
	}
}

// TestParseMaxConns covers the default, valid override, and rejection of bad
// PERSISTOR_DB_MAX_CONNS values.
func TestParseMaxConns(t *testing.T) {
	if n, err := parseMaxConns(""); err != nil || n != defaultDBMaxConns {
		t.Errorf("parseMaxConns(\"\") = %d, %v; want %d, nil", n, err, defaultDBMaxConns)
	}
	if n, err := parseMaxConns("16"); err != nil || n != 16 {
		t.Errorf("parseMaxConns(\"16\") = %d, %v; want 16, nil", n, err)
	}
	for _, bad := range []string{"0", "-1", "abc"} {
		if _, err := parseMaxConns(bad); err == nil {
			t.Errorf("parseMaxConns(%q) = nil error, want rejection", bad)
		}
	}
}

// TestLoadConfig_OIDC verifies the OIDC-only config resolution + validation. It
// uses t.Setenv (no DB, no network).
func TestLoadConfig_OIDC(t *testing.T) {
	base := func(t *testing.T) {
		t.Helper()
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("PERSISTOR_LISTEN_ADDR", "127.0.0.1:8088")
		t.Setenv("PERSISTOR_PUBLIC_URL", "")
		t.Setenv("PERSISTOR_OIDC_ISSUER", "")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "")
	}

	t.Run("missing oidc config is rejected", func(t *testing.T) {
		base(t)
		if _, err := loadConfig(); err == nil {
			t.Fatal("want error: OIDC is mandatory, no config given")
		}
	})

	t.Run("complete oidc config", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_OIDC_ISSUER", "https://issuer.example")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "persistor")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "https://issuer.example/.well-known/jwks.json")
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.publicURL != "http://127.0.0.1:8088" {
			t.Fatalf("publicURL = %q", cfg.publicURL)
		}
	})

	t.Run("non-https issuer is rejected", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_OIDC_ISSUER", "http://insecure.example")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "persistor")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "https://issuer.example/.well-known/jwks.json")
		if _, err := loadConfig(); err == nil {
			t.Fatal("want error for non-https issuer")
		}
	})

	t.Run("public url trailing slash trimmed", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_OIDC_ISSUER", "https://issuer.example")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "persistor")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "https://issuer.example/.well-known/jwks.json")
		t.Setenv("PERSISTOR_PUBLIC_URL", "https://persistor.example/")
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.publicURL != "https://persistor.example" {
			t.Fatalf("publicURL = %q, want trailing slash trimmed", cfg.publicURL)
		}
	})
}
