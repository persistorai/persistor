package main

import "testing"

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
