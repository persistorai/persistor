package index_test

import (
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

func TestDiscover_IncludesAndCore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "MEMORY.md", "# mem\n")
	writeFile(t, dir, "identity/SOUL.md", "# soul\n")
	writeFile(t, dir, "memory/daily/2026/06/a.md", "# a\n")
	writeFile(t, dir, "memory/daily/2026/06/b.md", "# b\n")
	writeFile(t, dir, "notes.txt", "ignored, not markdown\n")
	writeFile(t, dir, "other/skip.md", "# not included\n")

	root := index.Root{
		Name:      "scout",
		Dir:       dir,
		Includes:  []string{"MEMORY.md", "identity", "memory/daily"},
		CorePaths: []string{"MEMORY.md", "identity/SOUL.md"},
	}
	files, err := index.Discover(&root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d: %+v", len(files), files)
	}

	core := map[string]bool{}
	for _, f := range files {
		core[f.RelPath] = f.IsCore
		if f.RelPath == "other/skip.md" {
			t.Errorf("included a path outside Includes: %s", f.RelPath)
		}
	}
	if !core["MEMORY.md"] || !core["identity/SOUL.md"] {
		t.Errorf("expected core flags on MEMORY.md and identity/SOUL.md: %v", core)
	}
	if core["memory/daily/2026/06/a.md"] {
		t.Errorf("daily note should be tail, not core")
	}
}

func TestDiscover_MissingIncludeIsNotError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "MEMORY.md", "# mem\n")
	root := index.Root{Name: "scout", Dir: dir, Includes: []string{"MEMORY.md", "does-not-exist"}}
	files, err := index.Discover(&root)
	if err != nil {
		t.Fatalf("Discover should tolerate missing include: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
}

func TestDiscover_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b.md", "# b\n")
	writeFile(t, dir, "a.md", "# a\n")
	writeFile(t, dir, "c.md", "# c\n")
	root := index.Root{Name: "scout", Dir: dir}
	files, err := index.Discover(&root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	got := []string{files[0].RelPath, files[1].RelPath, files[2].RelPath}
	want := []string{"a.md", "b.md", "c.md"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHashContent_Stable(t *testing.T) {
	a := index.HashContent([]byte("hello"))
	b := index.HashContent([]byte("hello"))
	c := index.HashContent([]byte("world"))
	if a != b {
		t.Error("hash not stable for identical content")
	}
	if a == c {
		t.Error("hash collision for different content")
	}
	if len(a) != 64 {
		t.Errorf("sha256 hex len = %d, want 64", len(a))
	}
}
