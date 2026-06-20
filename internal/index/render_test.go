package index_test

import (
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestRenderParseRoundTrip is the pure (no-DB) core of the export backup story:
// RenderNote then ParseNote must recover the persisted fields and the body
// byte-for-byte, so an exported .md re-imports to the same note.
func TestRenderParseRoundTrip(t *testing.T) {
	original := index.Note{
		ID:         "scout:memory-atomic-foo",
		Kind:       "reference",
		Tier:       "core",
		Title:      "A Title: with punctuation & a colon",
		Body:       "# A Title: with punctuation & a colon\n\nSome prose with a [[link-to-bar]] inside.\n",
		Supersedes: "scout:old-note",
	}

	md, err := index.RenderNote(&original)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got := index.ParseNote("scout", "memory/atomic/foo.md", md, false)
	if got.ID != original.ID {
		t.Errorf("ID = %q, want %q", got.ID, original.ID)
	}
	if got.Kind != original.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, original.Kind)
	}
	if got.Tier != original.Tier {
		t.Errorf("Tier = %q, want %q", got.Tier, original.Tier)
	}
	if got.Title != original.Title {
		t.Errorf("Title = %q, want %q", got.Title, original.Title)
	}
	if got.Supersedes != original.Supersedes {
		t.Errorf("Supersedes = %q, want %q", got.Supersedes, original.Supersedes)
	}
	if got.Body != original.Body {
		t.Errorf("Body = %q, want %q", got.Body, original.Body)
	}
}

// TestRenderNoteOmitsEmptyOptional keeps optional frontmatter out of the file
// when unset, so a minimal note exports clean.
func TestRenderNoteOmitsEmptyOptional(t *testing.T) {
	md, err := index.RenderNote(&index.Note{
		ID: "scout:bare", Kind: "fact", Tier: "tail", Title: "Bare", Body: "Just body.\n",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, unwanted := range []string{"supersedes:", "links:"} {
		if strings.Contains(md, unwanted) {
			t.Errorf("rendered note unexpectedly contains %q:\n%s", unwanted, md)
		}
	}
}
