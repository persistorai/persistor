package index_test

import (
	"errors"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestResolveWriteID(t *testing.T) {
	roots := []index.Root{{Name: "demo", Dir: "/home/x/demo"}}
	notesDir := "/home/x/demo/memory/atomic"

	// Derived id: namespace + slug of the path relative to the ROOT.
	got, ok := index.ResolveWriteID(roots, notesDir, "claude/foo.md", "")
	if !ok {
		t.Fatal("resolve: not ok")
	}
	if want := "demo:memory-atomic-claude-foo"; got != want {
		t.Errorf("derived id = %q, want %q", got, want)
	}

	// Explicit id wins.
	got, ok = index.ResolveWriteID(roots, notesDir, "claude/foo.md", "demo:custom")
	if !ok || got != "demo:custom" {
		t.Errorf("explicit id = %q (ok %v), want demo:custom", got, ok)
	}

	// notesDir not under any root → not ok (callers skip the guard).
	if _, ok := index.ResolveWriteID(roots, "/tmp/elsewhere", "foo.md", ""); ok {
		t.Error("expected ok=false when notesDir is outside every root")
	}
}

func TestCheckSelfSupersede(t *testing.T) {
	roots := []index.Root{{Name: "demo", Dir: "/home/x/demo"}}
	notesDir := "/home/x/demo/memory/atomic"

	// Self-supersede (derived): supersedes == the id the write resolves to.
	err := index.CheckSelfSupersede(roots, notesDir, "claude/foo.md", "", "demo:memory-atomic-claude-foo")
	var selfErr *index.SelfSupersedeError
	if !errors.As(err, &selfErr) {
		t.Fatalf("want SelfSupersedeError, got %v", err)
	}

	// Self-supersede (explicit id).
	if err := index.CheckSelfSupersede(roots, notesDir, "foo.md", "demo:thing", "demo:thing"); err == nil {
		t.Error("want error for explicit self-supersede")
	}

	// Superseding a DIFFERENT note is fine (the legitimate fork path).
	if err := index.CheckSelfSupersede(roots, notesDir, "claude/foo-v2.md", "", "demo:memory-atomic-claude-foo"); err != nil {
		t.Errorf("superseding a different note should be allowed, got %v", err)
	}

	// No supersedes → no error.
	if err := index.CheckSelfSupersede(roots, notesDir, "foo.md", "", ""); err != nil {
		t.Errorf("empty supersedes should be a no-op, got %v", err)
	}

	// Unresolvable notesDir → guard skipped (nil), never blocks a write.
	if err := index.CheckSelfSupersede(roots, "/tmp/elsewhere", "foo.md", "", "demo:x"); err != nil {
		t.Errorf("unresolvable notesDir should skip the guard, got %v", err)
	}
}
