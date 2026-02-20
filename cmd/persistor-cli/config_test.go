package main

import (
	"os"
	"path/filepath"
	"testing"
)

// resetFlags restores global flag state after each test.
func resetFlags(t *testing.T) {
	t.Helper()
	orig := struct{ url, key, fmt string }{flagURL, flagKey, flagFmt}
	t.Cleanup(func() {
		flagURL = orig.url
		flagKey = orig.key
		flagFmt = orig.fmt
	})
}

// unsetEnv temporarily unsets an environment variable and restores it on cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, exists := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if exists {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// setEnv temporarily sets an environment variable and restores it on cleanup.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	prev, exists := os.LookupEnv(key)
	os.Setenv(key, val)
	t.Cleanup(func() {
		if exists {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// TestResolveConfigEnvURL verifies that PERSISTOR_URL overrides the default URL.
func TestResolveConfigEnvURL(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_API_KEY")
	setEnv(t, "PERSISTOR_URL", "http://env-server:9090")

	// Point HOME at a temp dir so there's no config file to interfere.
	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	flagURL = "http://localhost:3030" // default
	flagKey = ""
	resolveConfig()

	if flagURL != "http://env-server:9090" {
		t.Errorf("flagURL: got %q, want %q", flagURL, "http://env-server:9090")
	}
}

// TestResolveConfigEnvKey verifies that PERSISTOR_API_KEY sets the API key.
func TestResolveConfigEnvKey(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	setEnv(t, "PERSISTOR_API_KEY", "secret-key-from-env")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig()

	if flagKey != "secret-key-from-env" {
		t.Errorf("flagKey: got %q, want %q", flagKey, "secret-key-from-env")
	}
}

// TestResolveConfigFlagTakesPrecedenceOverEnv verifies that an explicit flag
// value is not overridden by the environment variable.
func TestResolveConfigFlagTakesPrecedenceOverEnv(t *testing.T) {
	resetFlags(t)
	setEnv(t, "PERSISTOR_URL", "http://env-server:9090")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	// Simulate flag being explicitly set to a non-default value.
	flagURL = "http://explicit-flag:1234"
	resolveConfig()

	if flagURL != "http://explicit-flag:1234" {
		t.Errorf("explicit flag should win; got %q", flagURL)
	}
}

// TestResolveConfigFlatYAML verifies that a flat-format config file (url/api_key
// at the top level) is read correctly.
func TestResolveConfigFlatYAML(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	unsetEnv(t, "PERSISTOR_API_KEY")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	cfgDir := filepath.Join(tmp, ".persistor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgContent := "url: http://from-file:8080\napi_key: file-key\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig()

	if flagURL != "http://from-file:8080" {
		t.Errorf("flagURL from flat config: got %q, want %q", flagURL, "http://from-file:8080")
	}
	if flagKey != "file-key" {
		t.Errorf("flagKey from flat config: got %q, want %q", flagKey, "file-key")
	}
}

// TestResolveConfigProfileYAML verifies that profile-based config is resolved
// using the active_profile key.
func TestResolveConfigProfileYAML(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	unsetEnv(t, "PERSISTOR_API_KEY")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	cfgDir := filepath.Join(tmp, ".persistor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgContent := `
active_profile: staging
profiles:
  default:
    url: http://default:3030
    api_key: default-key
  staging:
    url: http://staging:4040
    api_key: staging-key
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig()

	if flagURL != "http://staging:4040" {
		t.Errorf("flagURL from profile: got %q, want %q", flagURL, "http://staging:4040")
	}
	if flagKey != "staging-key" {
		t.Errorf("flagKey from profile: got %q, want %q", flagKey, "staging-key")
	}
}

// TestResolveConfigDefaultProfile verifies that when active_profile is empty
// the "default" profile is used.
func TestResolveConfigDefaultProfile(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	unsetEnv(t, "PERSISTOR_API_KEY")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	cfgDir := filepath.Join(tmp, ".persistor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgContent := `
profiles:
  default:
    url: http://default-profile:5050
    api_key: default-profile-key
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig()

	if flagURL != "http://default-profile:5050" {
		t.Errorf("flagURL from default profile: got %q, want %q", flagURL, "http://default-profile:5050")
	}
}

// TestResolveConfigMissingFile verifies that a missing config file is silently
// ignored and flag defaults are unchanged.
func TestResolveConfigMissingFile(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	unsetEnv(t, "PERSISTOR_API_KEY")

	// HOME has no .persistor directory.
	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig() // must not panic

	if flagURL != "http://localhost:3030" {
		t.Errorf("flagURL should stay default; got %q", flagURL)
	}
	if flagKey != "" {
		t.Errorf("flagKey should stay empty; got %q", flagKey)
	}
}

// TestResolveConfigInvalidYAML verifies that a malformed config file is
// silently ignored.
func TestResolveConfigInvalidYAML(t *testing.T) {
	resetFlags(t)
	unsetEnv(t, "PERSISTOR_URL")
	unsetEnv(t, "PERSISTOR_API_KEY")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	cfgDir := filepath.Join(tmp, ".persistor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(":::not-yaml:::"), 0o600); err != nil {
		t.Fatal(err)
	}

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig() // must not panic

	if flagURL != "http://localhost:3030" {
		t.Errorf("flagURL should stay default on bad YAML; got %q", flagURL)
	}
}

// TestResolveConfigEnvNotOverriddenByFile verifies that env vars take
// precedence over config file values.
func TestResolveConfigEnvNotOverriddenByFile(t *testing.T) {
	resetFlags(t)
	setEnv(t, "PERSISTOR_API_KEY", "env-wins-key")
	unsetEnv(t, "PERSISTOR_URL")

	tmp := t.TempDir()
	setEnv(t, "HOME", tmp)

	cfgDir := filepath.Join(tmp, ".persistor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgContent := "url: http://file:9000\napi_key: file-key\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	flagURL = "http://localhost:3030"
	flagKey = ""
	resolveConfig()

	// Env key should win over file key.
	if flagKey != "env-wins-key" {
		t.Errorf("flagKey should be env value; got %q", flagKey)
	}
}
