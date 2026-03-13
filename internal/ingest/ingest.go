package ingest

import (
	"context"
	"fmt"
	"io"
)

// IngestOpts configures the ingest pipeline.
type IngestOpts struct {
	Source  string
	DryRun bool
	ScanDir string
}

// IngestReport summarizes the results of ingestion.
type IngestReport struct {
	Chunks           int
	CreatedNodes     int
	UpdatedNodes     int
	SkippedNodes     int
	CreatedEdges     int
	SkippedEdges     int
	UnknownRelations int
	Errors           []string
}

// Ingester orchestrates the ingest pipeline: chunk, extract, write.
type Ingester struct {
	extractor *Extractor
	writer    *Writer
}

// NewIngester creates an Ingester with the given extractor and writer.
func NewIngester(extractor *Extractor, writer *Writer) *Ingester {
	return &Ingester{extractor: extractor, writer: writer}
}

// Ingest reads text, chunks it, extracts entities/relationships, and writes to the graph.
func (ing *Ingester) Ingest(
	ctx context.Context,
	input io.Reader,
	opts IngestOpts,
) (*IngestReport, error) {
	text, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	chunks := ChunkMarkdown(string(text), ChunkOpts{})
	report := &IngestReport{Chunks: len(chunks)}

	allEntities, allRels, allFacts := ing.extractAll(ctx, chunks, report)

	if opts.DryRun {
		return buildDryRunReport(report, allEntities, allRels), nil
	}

	return ing.writeAll(ctx, report, allEntities, allRels, allFacts)
}
