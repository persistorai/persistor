package main

import "testing"

// TestLoadConfig_AuthMode verifies the auth-mode config resolution + validation.
// It uses t.Setenv (no DB, no network).
func TestLoadConfig_AuthMode(t *testing.T) {
	base := func(t *testing.T) {
		t.Helper()
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("PERSISTOR_NOTES_DIR", "/tmp/notes")
		t.Setenv("PERSISTOR_LISTEN_ADDR", "127.0.0.1:8088")
		t.Setenv("PERSISTOR_AUTH_MODE", "")
		t.Setenv("PERSISTOR_PUBLIC_URL", "")
		t.Setenv("PERSISTOR_OIDC_ISSUER", "")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "")
	}

	t.Run("default is static", func(t *testing.T) {
		base(t)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.authMode != authModeStatic {
			t.Fatalf("authMode = %q, want static", cfg.authMode)
		}
		if cfg.publicURL != "http://127.0.0.1:8088" {
			t.Fatalf("publicURL = %q", cfg.publicURL)
		}
	})

	t.Run("oidc requires issuer/audience/jwks", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_AUTH_MODE", "oidc")
		if _, err := loadConfig(); err == nil {
			t.Fatal("want error for incomplete oidc config")
		}
	})

	t.Run("oidc complete", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_AUTH_MODE", "oidc")
		t.Setenv("PERSISTOR_OIDC_ISSUER", "https://issuer.example")
		t.Setenv("PERSISTOR_OIDC_AUDIENCE", "persistor")
		t.Setenv("PERSISTOR_OIDC_JWKS_URL", "https://issuer.example/.well-known/jwks.json")
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.authMode != authModeOIDC {
			t.Fatalf("authMode = %q, want oidc", cfg.authMode)
		}
	})

	t.Run("unknown mode rejected", func(t *testing.T) {
		base(t)
		t.Setenv("PERSISTOR_AUTH_MODE", "bogus")
		if _, err := loadConfig(); err == nil {
			t.Fatal("want error for unknown auth mode")
		}
	})

	t.Run("public url trailing slash trimmed", func(t *testing.T) {
		base(t)
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
