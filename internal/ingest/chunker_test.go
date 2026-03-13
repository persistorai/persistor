package ingest_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/persistorai/persistor/internal/ingest"
)

const testOverlapLines = 3

func TestChunkMarkdown_EmptyInput(t *testing.T) {
	chunks := ingest.ChunkMarkdown("", ingest.ChunkOpts{})
	assert.Empty(t, chunks)
}

func TestChunkMarkdown_ShortText(t *testing.T) {
	text := "Hello world\nThis is a test."
	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 2000})

	require.Len(t, chunks, 1)
	assert.Equal(t, text, chunks[0].Text)
	assert.Equal(t, 0, chunks[0].Index)
	assert.Equal(t, 1, chunks[0].StartLine)
	assert.Equal(t, 2, chunks[0].EndLine)
}

func TestChunkMarkdown_SplitsOnHeadings(t *testing.T) {
	sections := []string{
		buildSection("## Section 1", 60),
		buildSection("## Section 2", 60),
		buildSection("## Section 3", 60),
	}
	text := strings.Join(sections, "\n")

	// Use small MaxTokens so each section exceeds the threshold.
	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 200, Overlap: 50})

	require.Greater(t, len(chunks), 1, "should split into multiple chunks")

	for i, c := range chunks {
		assert.Equal(t, i, c.Index, "index should be sequential")
	}
}

func TestChunkMarkdown_HardSplitWithoutHeadings(t *testing.T) {
	// Build a long document with no headings.
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "This is a line of content that has no heading markers at all.")
	}
	text := strings.Join(lines, "\n")

	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 200, Overlap: 50})

	require.Greater(t, len(chunks), 1, "should hard-split without headings")

	for i, c := range chunks {
		assert.Equal(t, i, c.Index)
	}
}

func TestChunkMarkdown_OverlapLines(t *testing.T) {
	// Create sections that will cause splits.
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, strings.Repeat("word ", 20))
	}
	// Insert a heading to force a split.
	lines[100] = "## Middle Section"
	text := strings.Join(lines, "\n")

	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 300, Overlap: 50})

	require.Greater(t, len(chunks), 1)
	verifyOverlap(t, chunks)
}

func TestChunkMarkdown_LineNumbersAreOneBased(t *testing.T) {
	text := "Line one\nLine two\nLine three"
	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 2000})

	require.Len(t, chunks, 1)
	assert.Equal(t, 1, chunks[0].StartLine, "StartLine should be 1-based")
	assert.Equal(t, 3, chunks[0].EndLine)
}

func TestChunkMarkdown_IndexesAreSequential(t *testing.T) {
	sections := []string{
		buildSection("## A", 80),
		buildSection("## B", 80),
		buildSection("## C", 80),
		buildSection("## D", 80),
	}
	text := strings.Join(sections, "\n")

	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 150, Overlap: 30})

	for i, c := range chunks {
		assert.Equal(t, i, c.Index, "chunk index should be 0-based sequential")
	}
}

func TestChunkMarkdown_DefaultOpts(t *testing.T) {
	// Verify defaults are applied when zero values are passed.
	text := "short"
	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{})

	require.Len(t, chunks, 1)
	assert.Equal(t, text, chunks[0].Text)
}

func TestChunkMarkdown_StartEndLinesCover(t *testing.T) {
	// Verify that chunks cover the full document without gaps (ignoring overlap).
	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, strings.Repeat("text ", 15))
	}
	lines[100] = "## Break One"
	lines[200] = "## Break Two"
	text := strings.Join(lines, "\n")

	chunks := ingest.ChunkMarkdown(text, ingest.ChunkOpts{MaxTokens: 250, Overlap: 50})
	require.Greater(t, len(chunks), 1)

	// First chunk starts at line 1.
	assert.Equal(t, 1, chunks[0].StartLine)
	// Last chunk ends at the final line.
	assert.Equal(t, len(lines), chunks[len(chunks)-1].EndLine)
}

// buildSection creates a heading followed by n lines of filler text.
func buildSection(heading string, numLines int) string {
	lines := make([]string, 0, numLines+1)
	lines = append(lines, heading)
	for i := 0; i < numLines; i++ {
		lines = append(lines, "This is filler content for the section to increase size.")
	}
	return strings.Join(lines, "\n")
}

// verifyOverlap checks that the last 3 lines of chunk N appear at the start of chunk N+1.
func verifyOverlap(t *testing.T, chunks []ingest.Chunk) {
	t.Helper()
	for i := 0; i < len(chunks)-1; i++ {
		prevLines := strings.Split(chunks[i].Text, "\n")
		nextLines := strings.Split(chunks[i+1].Text, "\n")

		if len(prevLines) < testOverlapLines {
			continue
		}

		tail := prevLines[len(prevLines)-testOverlapLines:]
		if len(nextLines) < testOverlapLines {
			continue
		}
		head := nextLines[:testOverlapLines]

		assert.Equal(t, tail, head,
			"chunk %d tail should overlap with chunk %d head", i, i+1)
	}
}
