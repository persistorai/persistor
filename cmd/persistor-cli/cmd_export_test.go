package main

import (
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestExportRelPath checks that an export filename is derived safely from a
// note id: colons and slashes are flattened to hyphens, a .md suffix is added,
// and the result can never escape the --out directory (no separators survive,
// so the path-safety guard is defense-in-depth that an id can't trip).
func TestExportRelPath(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "namespaced id flattens colon and slash", id: "scout:foo/bar", want: "scout-foo-bar.md"},
		{name: "simple namespaced id", id: "claude:note", want: "claude-note.md"},
		{name: "bare id gets .md", id: "x", want: "x.md"},
		{name: "dot-dot in id stays inside, no escape", id: "..:evil", want: "..-evil.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exportRelPath(&index.Note{ID: tt.id})
			if err != nil {
				t.Fatalf("exportRelPath(%q) unexpected error: %v", tt.id, err)
			}
			if got != tt.want {
				t.Fatalf("exportRelPath(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
