package ingest

import (
	"context"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
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

// KnownEntityFetcher can retrieve existing entity names from the graph.
type KnownEntityFetcher interface {
	FetchTopEntityNames(ctx context.Context, limit int) ([]string, error)
}

// Ingester orchestrates the ingest pipeline: chunk, extract, write.
type Ingester struct {
	extractor    *Extractor
	writer       *Writer
	entityFetch  KnownEntityFetcher
}

// NewIngester creates an Ingester with the given extractor and writer.
func NewIngester(extractor *Extractor, writer *Writer, fetcher KnownEntityFetcher) *Ingester {
	return &Ingester{extractor: extractor, writer: writer, entityFetch: fetcher}
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

	// Fetch known entities to guide consistent naming
	var knownEntities []string
	if ing.entityFetch != nil {
		names, err := ing.entityFetch.FetchTopEntityNames(ctx, 50)
		if err != nil {
			// Non-fatal — continue without known entities
			logrus.Warnf("could not fetch known entities: %v", err)
		} else {
			knownEntities = names
		}
	}

	allEntities, allRels, allFacts := ing.extractAll(ctx, chunks, report, knownEntities)

	if opts.DryRun {
		return buildDryRunReport(report, allEntities, allRels), nil
	}

	return ing.writeAll(ctx, report, allEntities, allRels, allFacts)
}
