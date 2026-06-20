package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCollectMarkdown checks the file-discovery side of `persistor import`: a
// directory is walked recursively for .md files (with stable slash-relative
// paths), a single .md file imports directly, and a non-.md file is rejected.
func TestCollectMarkdown(t *testing.T) {
	dir := t.TempDir()
	mustWriteImport(t, dir, "top.md", "# Top")
	mustWriteImport(t, dir, "sub/deep.md", "# Deep")
	mustWriteImport(t, dir, "sub/notes.txt", "ignored")

	files, err := collectMarkdown(dir)
	if err != nil {
		t.Fatalf("collectMarkdown(dir): %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.rel] = true
	}
	if len(files) != 2 || !got["top.md"] || !got["sub/deep.md"] {
		t.Fatalf("collected %v, want top.md and sub/deep.md only", got)
	}

	// A single .md file imports directly, rel = base name.
	single, err := collectMarkdown(filepath.Join(dir, "top.md"))
	if err != nil || len(single) != 1 || single[0].rel != "top.md" {
		t.Fatalf("single-file collect = %+v (err %v), want one top.md", single, err)
	}

	// A non-.md file path is rejected.
	if _, err := collectMarkdown(filepath.Join(dir, "sub", "notes.txt")); err == nil {
		t.Fatal("collectMarkdown on a .txt file should error")
	}
}

func mustWriteImport(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
