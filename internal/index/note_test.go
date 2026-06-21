package index_test

import (
	"strings"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

func TestParseNote_Frontmatter(t *testing.T) {
	content := `---
id: my-note
kind: decision
tier: core
title: A Decision
supersedes: old-note
links: [other-note]
---

# A Decision

We decided to do the thing.
`
	n := index.ParseNote("demo", "memory/x.md", content, false)
	if n.ID != "my-note" {
		t.Errorf("ID = %q, want my-note", n.ID)
	}
	if n.Kind != "decision" {
		t.Errorf("Kind = %q, want decision", n.Kind)
	}
	if n.Tier != "core" {
		t.Errorf("Tier = %q, want core", n.Tier)
	}
	if n.Title != "A Decision" {
		t.Errorf("Title = %q", n.Title)
	}
	if n.Supersedes != "old-note" {
		t.Errorf("Supersedes = %q", n.Supersedes)
	}
	// An unknown `links:` frontmatter key (the retired placeholder) is tolerated:
	// ParseNote ignores it rather than failing.
	mustContain(t, n.Body, "decided to do the thing")
}

func TestParseNote_DerivedDefaults(t *testing.T) {
	content := "# Daily Note\n\nStuff happened today.\n"
	n := index.ParseNote("demo", "memory/daily/2026/06/2026-06-17.md", content, false)
	want := "demo:memory-daily-2026-06-2026-06-17"
	if n.ID != want {
		t.Errorf("derived ID = %q, want %q", n.ID, want)
	}
	if n.Kind != "fact" {
		t.Errorf("default Kind = %q, want fact", n.Kind)
	}
	if n.Tier != "tail" {
		t.Errorf("default Tier = %q, want tail", n.Tier)
	}
	if n.Title != "Daily Note" {
		t.Errorf("Title = %q, want 'Daily Note'", n.Title)
	}
}

func TestParseNote_CoreTier(t *testing.T) {
	n := index.ParseNote("demo", "identity/SOUL.md", "# Soul\n\nWho I am.\n", true)
	if n.Tier != "core" {
		t.Errorf("isCore file Tier = %q, want core", n.Tier)
	}
}

func TestParseNote_NamespacePreventsCollision(t *testing.T) {
	a := index.ParseNote("demo", "MEMORY.md", "# Mem\n\nA\n", true)
	b := index.ParseNote("claude", "MEMORY.md", "# Mem\n\nB\n", true)
	if a.ID == b.ID {
		t.Errorf("namespaces did not prevent id collision: both %q", a.ID)
	}
}

func TestParseNote_NoFrontmatterButLeadingDashes(t *testing.T) {
	// A horizontal rule mid-doc must not be mistaken for frontmatter.
	content := "Some intro.\n\n---\n\nMore text.\n"
	n := index.ParseNote("demo", "x.md", content, false)
	if !strings.Contains(n.Body, "Some intro") || !strings.Contains(n.Body, "More text") {
		t.Errorf("body lost content: %q", n.Body)
	}
}

func TestParseNote_MalformedFrontmatterIsNonFatal(t *testing.T) {
	content := "---\n: : : not yaml\n---\n\nBody survives.\n"
	n := index.ParseNote("demo", "x.md", content, false)
	mustContain(t, n.Body, "Body survives")
	if n.ID == "" {
		t.Error("expected derived ID when frontmatter id missing/malformed")
	}
}
