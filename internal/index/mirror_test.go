package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestMirrorMarkdown_CopiesAndPrunes(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, src, "a.md", "# a\n")
	writeFile(t, src, "sub/b.md", "# b\n")
	writeFile(t, src, "ignore.txt", "not markdown\n")

	if err := index.MirrorMarkdown(src, dest); err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.md")); err != nil {
		t.Errorf("a.md not mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "b.md")); err != nil {
		t.Errorf("sub/b.md not mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "ignore.txt")); !os.IsNotExist(err) {
		t.Errorf("non-markdown should not be mirrored")
	}

	// Remove a source file, re-mirror: it should be pruned from dest.
	if err := os.Remove(filepath.Join(src, "sub", "b.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := index.MirrorMarkdown(src, dest); err != nil {
		t.Fatalf("re-mirror: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "b.md")); !os.IsNotExist(err) {
		t.Errorf("deleted source not pruned from mirror")
	}
	if _, err := os.Stat(filepath.Join(dest, "a.md")); err != nil {
		t.Errorf("surviving file wrongly pruned: %v", err)
	}
}

func TestMirrorMarkdown_MissingSourceIsNoOp(t *testing.T) {
	dest := t.TempDir()
	if err := index.MirrorMarkdown(filepath.Join(t.TempDir(), "nope"), dest); err != nil {
		t.Errorf("missing source should be a no-op, got %v", err)
	}
}
