package main

import (
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestExportRelPath checks the path-safety guard that keeps an export from
// writing outside its --out directory: absolute paths and "../" escapes are
// rejected, and a missing/odd source_path falls back to an id-derived filename.
func TestExportRelPath(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		sourcePath string
		want       string
		wantErr    bool
	}{
		{name: "id fallback sanitizes colons and slashes", id: "scout:foo/bar", want: "scout-foo-bar.md"},
		{name: "id fallback adds .md", id: "claude:note", want: "claude-note.md"},
		{name: "well-behaved relative path kept", id: "x", sourcePath: "memory/atomic/x.md", want: "memory/atomic/x.md"},
		{name: "relative path gets .md suffix", id: "x", sourcePath: "memory/atomic/x", want: "memory/atomic/x.md"},
		{name: "interior dot-dot cleaned but stays inside", id: "x", sourcePath: "a/../b.md", want: "b.md"},
		{name: "absolute path rejected", id: "x", sourcePath: "/etc/passwd", wantErr: true},
		{name: "parent escape rejected", id: "x", sourcePath: "../../etc/passwd", wantErr: true},
		{name: "bare dot-dot rejected", id: "x", sourcePath: "..", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exportRelPath(&index.Note{ID: tt.id, SourcePath: tt.sourcePath})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("exportRelPath(%q, %q) = %q, want error", tt.id, tt.sourcePath, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("exportRelPath(%q, %q) unexpected error: %v", tt.id, tt.sourcePath, err)
			}
			if got != tt.want {
				t.Fatalf("exportRelPath(%q, %q) = %q, want %q", tt.id, tt.sourcePath, got, tt.want)
			}
		})
	}
}
