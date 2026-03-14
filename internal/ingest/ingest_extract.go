package ingest

import (
	"context"
	"fmt"
)

// extractAll runs the extractor on each chunk, collecting all results.
func (ing *Ingester) extractAll(
	ctx context.Context,
	chunks []Chunk,
	report *IngestReport,
	knownEntities []string,
) ([]ExtractedEntity, []ExtractedRelationship, []ExtractedFact) {
	var allEntities []ExtractedEntity
	var allRels []ExtractedRelationship
	var allFacts []ExtractedFact

	for _, chunk := range chunks {
		entities, rels, facts := ing.extractChunk(ctx, chunk, report, knownEntities)
		allEntities = append(allEntities, entities...)
		allRels = append(allRels, rels...)
		allFacts = append(allFacts, facts...)
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
	result, err := ing.extractor.Extract(ctx, chunk.Text, knownEntities...)
	if err != nil {
		report.Errors = append(report.Errors,
			fmt.Sprintf("chunk %d: %v", chunk.Index, err))
		return nil, nil, nil
	}

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
