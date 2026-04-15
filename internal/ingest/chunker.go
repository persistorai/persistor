package ingest

import (
	"strconv"
	"strings"
)

const (
	defaultMaxTokens = 2000
	defaultOverlap   = 200
	overlapLines     = 3
	bytesPerToken    = 4
)

// ChunkOpts configures the chunking behavior.
type ChunkOpts struct {
	MaxTokens int // default 2000
	Overlap   int // default 200 (in tokens)
}

// Chunk represents a section of the input document.
type Chunk struct {
	Text      string
	Index     int
	StartLine int
	EndLine   int
}

// ChunkMarkdown splits markdown text into chunks, preferring ## heading boundaries.
func ChunkMarkdown(text string, opts ChunkOpts) []Chunk {
	if text == "" {
		return nil
	}
	opts = applyDefaults(opts)
	lines := strings.Split(text, "\n")

	if estimateTokens(text) <= opts.MaxTokens {
		return []Chunk{{Text: text, Index: 0, StartLine: 1, EndLine: len(lines)}}
	}

	return buildChunks(lines, opts)
}

func applyDefaults(opts ChunkOpts) ChunkOpts {
	if opts.MaxTokens == 0 {
		if override := envOrDefault("PERSISTOR_INGEST_MAX_TOKENS", ""); override != "" {
			if parsed, err := strconv.Atoi(override); err == nil && parsed > 0 {
				opts.MaxTokens = parsed
			} else {
				opts.MaxTokens = defaultMaxTokens
			}
		} else {
			opts.MaxTokens = defaultMaxTokens
		}
	}
	if opts.Overlap == 0 {
		opts.Overlap = defaultOverlap
	}
	return opts
}

func estimateTokens(text string) int {
	return len(text) / bytesPerToken
}

// buildChunks walks lines and splits on heading boundaries or hard limits.
func buildChunks(lines []string, opts ChunkOpts) []Chunk {
	var chunks []Chunk
	var overlapPrefix []string

	start := 0
	for start < len(lines) {
		end, chunkLines := collectChunk(lines, start, overlapPrefix, opts)
		chunk := assembleChunk(chunkLines, len(chunks), start, end)
		chunks = append(chunks, chunk)
		overlapPrefix = extractOverlap(lines, start, end)
		start = end
	}

	return chunks
}

// collectChunk gathers lines for one chunk, splitting at headings or hard limit.
func collectChunk(lines []string, start int, overlap []string, opts ChunkOpts) (endLine int, chunkLines []string) {
	chunkLines = make([]string, 0, len(overlap)+opts.MaxTokens)
	chunkLines = append(chunkLines, overlap...)

	hardLimit := opts.MaxTokens * 2
	endLine = start

	for endLine < len(lines) {
		line := lines[endLine]
		if shouldSplitAtHeading(line, chunkLines, overlap, opts) {
			break
		}
		chunkLines = append(chunkLines, line)
		endLine++
		if shouldHardSplit(chunkLines, hardLimit) {
			break
		}
	}

	if endLine == start && start < len(lines) {
		return hardSplitLine(lines, start, overlap, opts.MaxTokens)
	}

	return endLine, chunkLines
}

func shouldSplitAtHeading(line string, current, overlap []string, opts ChunkOpts) bool {
	if !isHeading(line) {
		return false
	}
	// Only split if we have content beyond the overlap prefix.
	contentLines := len(current) - len(overlap)
	if contentLines <= 0 {
		return false
	}
	text := strings.Join(current, "\n")
	return estimateTokens(text) >= opts.MaxTokens
}

func shouldHardSplit(current []string, hardLimit int) bool {
	text := strings.Join(current, "\n")
	return estimateTokens(text) >= hardLimit
}

func isHeading(line string) bool {
	return strings.HasPrefix(line, "## ")
}

// hardSplitLine handles very long lines with no newlines by splitting by byte count.
func hardSplitLine(lines []string, start int, overlap []string, maxTokens int) (endLine int, result []string) {
	line := lines[start]
	maxBytes := maxTokens * bytesPerToken

	result = make([]string, 0, len(overlap)+1)
	result = append(result, overlap...)

	if len(line) <= maxBytes {
		result = append(result, line)
		return start + 1, result
	}

	// Consume the full line even if it exceeds maxBytes.
	result = append(result, line)
	return start + 1, result
}

// assembleChunk creates a Chunk from collected lines.
func assembleChunk(chunkLines []string, index, start, end int) Chunk {
	return Chunk{
		Text:      strings.Join(chunkLines, "\n"),
		Index:     index,
		StartLine: start + 1,
		EndLine:   end,
	}
}

// extractOverlap returns the last N lines from the range [start, end) for context.
func extractOverlap(lines []string, start, end int) []string {
	if end <= start {
		return nil
	}
	overlapStart := end - overlapLines
	if overlapStart < start {
		overlapStart = start
	}
	result := make([]string, end-overlapStart)
	copy(result, lines[overlapStart:end])
	return result
}
