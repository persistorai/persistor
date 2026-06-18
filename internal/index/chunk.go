package index

import "strings"

// DefaultChunkWords targets roughly ~512 tokens per chunk. English prose averages ~1.3 tokens/word, so ~400
// words approximates the budget. Tunable via later.
const DefaultChunkWords = 400

// Chunk splits a note's searchable text into chunks of about targetWords words,
// preferring to break at markdown headings and paragraph boundaries so a chunk
// stays topically coherent. The note title is prepended so title terms are
// always searchable. Always returns at least one chunk (possibly just the
// title) so every note is represented in the index.
func Chunk(title, body string, targetWords int) []string {
	if targetWords <= 0 {
		targetWords = DefaultChunkWords
	}

	var searchText strings.Builder
	if title != "" {
		searchText.WriteString(title)
		searchText.WriteString("\n\n")
	}
	searchText.WriteString(body)

	blocks := splitBlocks(searchText.String())
	chunks := packBlocks(blocks, targetWords)

	if len(chunks) == 0 {
		if title != "" {
			return []string{title}
		}
		return []string{""}
	}
	return chunks
}

// splitBlocks breaks text into atomic blocks: a markdown heading starts a new
// block, and blank lines separate paragraphs.
func splitBlocks(text string) []string {
	var blocks []string
	var cur []string

	flush := func() {
		if len(cur) > 0 {
			block := strings.TrimRight(strings.Join(cur, "\n"), "\n")
			if strings.TrimSpace(block) != "" {
				blocks = append(blocks, block)
			}
			cur = nil
		}
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"): // heading: start a fresh block
			flush()
			cur = append(cur, line)
		case trimmed == "": // blank line: paragraph boundary
			flush()
		default:
			cur = append(cur, line)
		}
	}
	flush()
	return blocks
}

// packBlocks greedily packs blocks into chunks up to targetWords. A single block
// larger than the target becomes its own (oversized) chunk rather than being
// split mid-paragraph — coherence beats a hard word cap.
func packBlocks(blocks []string, targetWords int) []string {
	chunks := make([]string, 0, len(blocks))
	cur := make([]string, 0, len(blocks))
	curWords := 0

	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, strings.Join(cur, "\n\n"))
			cur = nil
			curWords = 0
		}
	}

	for _, b := range blocks {
		bw := len(strings.Fields(b))
		if curWords > 0 && curWords+bw > targetWords {
			flush()
		}
		cur = append(cur, b)
		curWords += bw
	}
	flush()
	return chunks
}
