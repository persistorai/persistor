package index_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

// writeFile writes content to dir/rel, creating parent directories. Shared by
// the tests that still touch the filesystem (e.g. the SeedQuery README probe).
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

// seedMarkdown parses one markdown note (frontmatter + body) the way the old file
// indexer did and writes it PG-native, so tests can seed a corpus without a
// filesystem. Derived ids are prefixed with the "syn" synthetic namespace (e.g.
// "syn:aurora"); core marks the always-loaded Core tier. Callers reconcile
// supersession afterward.
func seedMarkdown(t *testing.T, store *index.Store, tenantID, rel, content string, core bool) {
	t.Helper()
	n := index.ParseNote("syn", rel, content, core)
	if _, err := store.WriteNote(context.Background(), tenantID, &index.PGNoteInput{
		ID: n.ID, Kind: n.Kind, Tier: n.Tier, Title: n.Title, Body: n.Body,
		Supersedes: n.Supersedes, Surface: "test",
	}, 0); err != nil {
		t.Fatalf("seed %s: %v", rel, err)
	}
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected %q to contain %q", s, sub)
	}
}
