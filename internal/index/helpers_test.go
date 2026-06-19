package index_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes content to dir/rel, creating parent directories. Shared by
// the index package's tests.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// rmFile removes dir/rel, failing the test if it cannot. Shared by tests that
// exercise the file-sync delete path.
func rmFile(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}
}

// writeSymlink creates a symlink at dir/rel pointing to target, creating parent
// directories. Shared by tests that exercise the include path guardrails.
func writeSymlink(t *testing.T, dir, rel, target string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatalf("symlink %s -> %s: %v", rel, target, err)
	}
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected %q to contain %q", s, sub)
	}
}
