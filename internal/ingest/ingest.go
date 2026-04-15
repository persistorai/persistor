package ingest

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
)

// IngestOpts configures the ingest pipeline.
type IngestOpts struct {
	Source      string
	DryRun      bool
	ScanDir     string
	ChunkTokens int
	Progress    ProgressSink
}

// IngestReport summarizes the results of ingestion.
type IngestReport struct {
	Chunks                 int
	ExtractedEntities      int
	ExtractedRelationships int
	ExtractedFacts         int
	CreatedNodes           int
	UpdatedNodes           int
	SkippedNodes           int
	CreatedEdges           int
	SkippedEdges           int
	UnknownRelations       int
	CreatedEpisodes        int
	CreatedEvents          int
	Errors                 []string
	StartedAt              time.Time
	CompletedAt            time.Time
	ExtractDuration        time.Duration
	FinalizeDuration       time.Duration
	WriteDuration          time.Duration
	TotalDuration          time.Duration
	Diagnostics            IngestDiagnostics
}

// KnownEntityFetcher can retrieve existing entity names from the graph.
type KnownEntityFetcher interface {
	FetchTopEntityNames(ctx context.Context, limit int) ([]string, error)
}

// Ingester orchestrates the ingest pipeline: chunk, extract, write.
type Ingester struct {
	extractor   *Extractor
	writer      *Writer
	entityFetch KnownEntityFetcher
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
	startedAt := time.Now()
	text, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	chunkOpts := ChunkOpts{MaxTokens: opts.ChunkTokens}
	chunks := ChunkMarkdown(string(text), chunkOpts)
	report := &IngestReport{Chunks: len(chunks), StartedAt: startedAt}
	diagnostics := newDiagnosticsCollector()
	ing.writer = ing.writer.WithDiagnostics(diagnostics)

	emitProgress(opts.Progress, ProgressEvent{
		Source:      opts.Source,
		Stage:       "chunked",
		TotalChunks: report.Chunks,
		Elapsed:     time.Since(startedAt),
	})

	// Fetch known entities to guide consistent naming
	var knownEntities []string
	if envOrDefault("PERSISTOR_INGEST_DISABLE_KNOWN_ENTITIES", "") == "1" {
		knownEntities = nil
	} else if ing.entityFetch != nil {
		names, err := ing.entityFetch.FetchTopEntityNames(ctx, 50)
		if err != nil {
			// Non-fatal, continue without known entities.
			logrus.Warnf("could not fetch known entities: %v", err)
		} else {
			knownEntities = names
		}
	}

	extractStarted := time.Now()
	allEntities, allRels, allFacts := ing.extractAll(ctx, chunks, report, knownEntities, opts)
	report.ExtractDuration = time.Since(extractStarted)
	report.ExtractedEntities = len(allEntities)
	report.ExtractedRelationships = len(allRels)
	report.ExtractedFacts = len(allFacts)

	finalizeStarted := time.Now()
	allEntities, allRels, allFacts = FinalizeExtraction(allEntities, allRels, allFacts, knownEntities, string(text))
	report.FinalizeDuration = time.Since(finalizeStarted)
	report.ExtractedEntities = len(allEntities)
	report.ExtractedRelationships = len(allRels)
	report.ExtractedFacts = len(allFacts)

	emitProgress(opts.Progress, ProgressEvent{
		Source:        opts.Source,
		Stage:         "finalized",
		TotalChunks:   report.Chunks,
		Entities:      report.ExtractedEntities,
		Relationships: report.ExtractedRelationships,
		Facts:         report.ExtractedFacts,
		Errors:        len(report.Errors),
		Elapsed:       time.Since(startedAt),
		StageElapsed:  report.FinalizeDuration,
	})

	if opts.DryRun {
		report = buildDryRunReport(report, allEntities, allRels)
		report.CompletedAt = time.Now()
		report.TotalDuration = report.CompletedAt.Sub(startedAt)
		report.Diagnostics = diagnostics.snapshot()
		emitProgress(opts.Progress, ProgressEvent{
			Source:        opts.Source,
			Stage:         "done",
			TotalChunks:   report.Chunks,
			Entities:      report.ExtractedEntities,
			Relationships: report.ExtractedRelationships,
			Facts:         report.ExtractedFacts,
			CreatedNodes:  report.CreatedNodes,
			CreatedEdges:  report.CreatedEdges,
			Errors:        len(report.Errors),
			Alerts:        append([]string(nil), report.Diagnostics.Alerts...),
			Elapsed:       report.TotalDuration,
		})
		return report, nil
	}

	report, err = ing.writeAll(ctx, report, allEntities, allRels, allFacts)
	report.CompletedAt = time.Now()
	report.TotalDuration = report.CompletedAt.Sub(startedAt)
	report.Diagnostics = diagnostics.snapshot()
	emitProgress(opts.Progress, ProgressEvent{
		Source:        opts.Source,
		Stage:         "done",
		TotalChunks:   report.Chunks,
		Entities:      report.ExtractedEntities,
		Relationships: report.ExtractedRelationships,
		Facts:         report.ExtractedFacts,
		CreatedNodes:  report.CreatedNodes,
		UpdatedNodes:  report.UpdatedNodes,
		SkippedNodes:  report.SkippedNodes,
		CreatedEdges:  report.CreatedEdges,
		SkippedEdges:  report.SkippedEdges,
		Errors:        len(report.Errors),
		Alerts:        append([]string(nil), report.Diagnostics.Alerts...),
		Elapsed:       report.TotalDuration,
	})
	if err != nil {
		return report, err
	}

	return report, nil
}
