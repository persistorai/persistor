package index_test

import (
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

func TestChunk_TitlePrepended(t *testing.T) {
	chunks := index.Chunk("My Title", "Short body.\n", index.DefaultChunkWords)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "My Title") {
		t.Errorf("title not searchable in chunk: %q", chunks[0])
	}
}

func TestChunk_AlwaysAtLeastOne(t *testing.T) {
	if got := index.Chunk("", "", index.DefaultChunkWords); len(got) != 1 {
		t.Errorf("empty note: want 1 chunk, got %d", len(got))
	}
	if got := index.Chunk("Only Title", "", index.DefaultChunkWords); len(got) != 1 || !strings.Contains(got[0], "Only Title") {
		t.Errorf("title-only: got %v", got)
	}
}

func TestChunk_SplitsLargeBodyByWordBudget(t *testing.T) {
	// Build many short paragraphs that exceed a small budget.
	var b strings.Builder
	for range 50 {
		b.WriteString("alpha bravo charlie delta echo foxtrot golf hotel india\n\n")
	}
	chunks := index.Chunk("T", b.String(), 50)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks under a 50-word budget, got %d", len(chunks))
	}
	// No content lost: every paragraph's text appears somewhere.
	joined := strings.Join(chunks, "\n")
	if strings.Count(joined, "alpha bravo charlie") != 50 {
		t.Errorf("lost paragraphs: found %d of 50", strings.Count(joined, "alpha bravo charlie"))
	}
}

func TestChunk_HeadingStartsNewChunk(t *testing.T) {
	body := "## Section A\n\nalpha content here\n\n## Section B\n\nbeta content here\n"
	chunks := index.Chunk("", body, 5) // tiny budget forces a break
	if len(chunks) < 2 {
		t.Fatalf("expected headings to split, got %d chunks", len(chunks))
	}
}
