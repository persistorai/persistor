package ingest

import (
	"context"
)

// BenchmarkExtract runs the local extraction pipeline without writing to the KG.
func BenchmarkExtract(ctx context.Context, extractor *Extractor, text string, chunkTokens int, knownEntities []string) *ExtractionResult {
	chunks := ChunkMarkdown(text, ChunkOpts{MaxTokens: chunkTokens})
	var allEntities []ExtractedEntity
	var allRels []ExtractedRelationship
	var allFacts []ExtractedFact

	for _, chunk := range chunks {
		result, err := extractor.ExtractWithRetry(ctx, chunk.Text, knownEntities...)
		if err != nil {
			continue
		}
		result = PostProcessExtraction(result, knownEntities)
		allEntities = append(allEntities, result.Entities...)
		allRels = append(allRels, result.Relationships...)
		allFacts = append(allFacts, result.Facts...)
	}

	entities, rels, facts := FinalizeExtraction(allEntities, allRels, allFacts, knownEntities, text)
	return &ExtractionResult{
		Entities:      entities,
		Relationships: rels,
		Facts:         facts,
	}
}
