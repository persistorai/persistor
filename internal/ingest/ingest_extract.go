package ingest

import (
	"context"
	"fmt"
	"time"
)

// extractAll runs the extractor on each chunk, collecting all results.
func (ing *Ingester) extractAll(
	ctx context.Context,
	chunks []Chunk,
	report *IngestReport,
	knownEntities []string,
	opts IngestOpts,
) ([]ExtractedEntity, []ExtractedRelationship, []ExtractedFact) {
	var allEntities []ExtractedEntity
	var allRels []ExtractedRelationship
	var allFacts []ExtractedFact

	for _, chunk := range chunks {
		eventStart := time.Now()
		emitProgress(opts.Progress, ProgressEvent{
			Source:      opts.Source,
			Stage:       "extracting",
			ChunkIndex:  chunk.Index + 1,
			TotalChunks: len(chunks),
			Elapsed:     time.Since(report.StartedAt),
		})

		entities, rels, facts := ing.extractChunk(ctx, chunk, report, knownEntities)
		ing.writer.recordChunkThroughput(chunk.Index+1, time.Since(eventStart), len(entities), len(rels), len(facts))
		allEntities = append(allEntities, entities...)
		allRels = append(allRels, rels...)
		allFacts = append(allFacts, facts...)

		emitProgress(opts.Progress, ProgressEvent{
			Source:        opts.Source,
			Stage:         "extracted",
			ChunkIndex:    chunk.Index + 1,
			TotalChunks:   len(chunks),
			Entities:      len(allEntities),
			Relationships: len(allRels),
			Facts:         len(allFacts),
			Errors:        len(report.Errors),
			Elapsed:       time.Since(report.StartedAt),
			StageElapsed:  time.Since(eventStart),
		})
	}

	return allEntities, allRels, allFacts
}

// extractChunk extracts from a single chunk, logging errors to the report.
func (ing *Ingester) extractChunk(
	ctx context.Context,
	chunk Chunk,
	report *IngestReport,
	knownEntities []string,
) ([]ExtractedEntity, []ExtractedRelationship, []ExtractedFact) {
	result, err := ing.extractor.ExtractWithRetry(ctx, chunk.Text, knownEntities...)
	if err != nil {
		if isParseError(err) {
			ing.writer.recordParseFailure()
		}
		ing.writer.recordAPIFailure(err)
		report.Errors = append(report.Errors,
			fmt.Sprintf("chunk %d: %v", chunk.Index, err))
		return nil, nil, nil
	}

	result = PostProcessExtraction(result, knownEntities)
	return result.Entities, result.Relationships, result.Facts
}

// buildDryRunReport populates counts without writing.
func buildDryRunReport(
	report *IngestReport,
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
) *IngestReport {
	report.CreatedNodes = len(entities)
	report.CreatedEdges = len(rels)
	return report
}
