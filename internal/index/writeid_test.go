package index_test

import (
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestDeriveNoteID(t *testing.T) {
	// Explicit id wins and is returned as-is.
	got, err := index.DeriveNoteID("demo", "ignored.md", "demo:custom")
	if err != nil || got != "demo:custom" {
		t.Fatalf("explicit id = %q (err %v), want demo:custom", got, err)
	}

	// Derived id: namespace + slug of the path.
	got, err = index.DeriveNoteID("demo", "claude/foo.md", "")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if want := "demo:claude-foo"; got != want {
		t.Errorf("derived id = %q, want %q", got, want)
	}

	// No namespace → bare slug.
	got, err = index.DeriveNoteID("", "memory/widget.md", "")
	if err != nil || got != "memory-widget" {
		t.Errorf("derived id = %q (err %v), want memory-widget", got, err)
	}
}

func TestDeriveNoteID_Rejects(t *testing.T) {
	// A path without a .md extension cannot derive an id.
	if _, err := index.DeriveNoteID("demo", "notes/foo.txt", ""); err == nil {
		t.Error("want error for non-.md path")
	}
	// Path traversal is rejected.
	if _, err := index.DeriveNoteID("demo", "../escape.md", ""); err == nil {
		t.Error("want error for traversal path")
	}
	// An explicit id with illegal characters is rejected.
	if _, err := index.DeriveNoteID("", "", "Bad ID!"); err == nil {
		t.Error("want error for invalid explicit id")
	}
}
