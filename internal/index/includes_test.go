package index_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

// wantDefaultIncludes mirrors the hardcoded default so the test fails loudly if
// the backward-compatible list ever changes shape unintentionally.
var wantDefaultIncludes = []string{
	"memory/MEMORY.md",
	"memory/daily",
	"memory/atomic",
	"identity",
	"TOOLS.md",
	"AGENTS.md",
}

func TestNotesRoot_DefaultWhenEnvUnset(t *testing.T) {
	t.Setenv(index.NotesIncludesEnv, "")
	dir := t.TempDir()

	root, err := index.NotesRoot(dir)
	if err != nil {
		t.Fatalf("NotesRoot: %v", err)
	}
	if !reflect.DeepEqual(root.Includes, wantDefaultIncludes) {
		t.Errorf("default includes = %v, want %v", root.Includes, wantDefaultIncludes)
	}
}

func TestNotesRoot_CustomValidList(t *testing.T) {
	t.Setenv(index.NotesIncludesEnv, "memory/MEMORY.md:memory/daily:memory/reference")
	dir := t.TempDir()

	root, err := index.NotesRoot(dir)
	if err != nil {
		t.Fatalf("NotesRoot: %v", err)
	}
	want := []string{"memory/MEMORY.md", "memory/daily", "memory/reference"}
	if !reflect.DeepEqual(root.Includes, want) {
		t.Errorf("custom includes = %v, want %v", root.Includes, want)
	}
}

func TestNotesRoot_RejectsGuardrailViolations(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"root slash", "/"},
		{"absolute etc passwd", "/etc/passwd"},
		{"parent escape", "../secrets"},
		{"clean parent escape", "memory/../../etc"},
		{"double parent escape", "foo/../../bar"},
		{"empty entry", ":"},
		{"dot entry", "."},
		{"leading and trailing whitespace", " memory/daily "},
		// A valid entry alongside a bad one must still reject the whole list.
		{"valid then escape", "memory/daily:../secrets"},
		// Windows drive paths contain ":", the list separator, so they cannot be
		// a single entry; UNC paths (no colon) exercise the volume guardrail.
		{"unc path", `\\host\share`},
	}

	dir := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(index.NotesIncludesEnv, tt.value)

			_, err := index.NotesRoot(dir)
			if err == nil {
				t.Fatalf("NotesRoot(%q) = nil error, want rejection", tt.value)
			}
			var invalid *index.InvalidIncludeError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %T (%v), want *index.InvalidIncludeError", err, err)
			}
		})
	}
}

func TestNotesRoot_RejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	// A symlink inside the root whose target lives outside the root must be
	// rejected even though the entry itself is a clean relative subpath.
	linkRel := "memory/leak"
	writeSymlink(t, dir, linkRel, outside)

	t.Setenv(index.NotesIncludesEnv, linkRel)
	_, err := index.NotesRoot(dir)
	if err == nil {
		t.Fatalf("NotesRoot with escaping symlink = nil error, want rejection")
	}
	var invalid *index.InvalidIncludeError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %T (%v), want *index.InvalidIncludeError", err, err)
	}
}

func TestNotesRoot_AllowsMissingInclude(t *testing.T) {
	// Shape is valid and the path simply doesn't exist yet — not an error.
	t.Setenv(index.NotesIncludesEnv, "memory/reference:memory/runbooks")
	dir := t.TempDir()

	root, err := index.NotesRoot(dir)
	if err != nil {
		t.Fatalf("NotesRoot: %v", err)
	}
	want := []string{"memory/reference", "memory/runbooks"}
	if !reflect.DeepEqual(root.Includes, want) {
		t.Errorf("includes = %v, want %v", root.Includes, want)
	}
}
