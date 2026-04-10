package ingest

import (
	"context"
	"fmt"
)

// writeAll writes entities, relationships, and facts to the graph.
func (ing *Ingester) writeAll(
	ctx context.Context,
	report *IngestReport,
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
	facts []ExtractedFact,
) (*IngestReport, error) {
	nodeMap, err := ing.writeEntities(ctx, report, entities)
	if err != nil {
		return report, err
	}

	if err := ing.writeRelationships(ctx, report, rels, nodeMap); err != nil {
		return report, err
	}

	if err := ing.writeFacts(ctx, facts, nodeMap, report); err != nil {
		return report, err
	}

	if err := ing.writeEpisodic(ctx, entities, rels, nodeMap, report); err != nil {
		return report, err
	}

	return report, nil
}

// writeEntities writes entities and merges the report.
func (ing *Ingester) writeEntities(
	ctx context.Context,
	report *IngestReport,
	entities []ExtractedEntity,
) (map[string]string, error) {
	wr, nodeMap, err := ing.writer.WriteEntities(ctx, entities)
	if err != nil {
		return nil, fmt.Errorf("writing entities: %w", err)
	}

	report.CreatedNodes = wr.CreatedNodes
	report.UpdatedNodes = wr.UpdatedNodes
	report.SkippedNodes = wr.SkippedNodes

	return nodeMap, nil
}

// writeRelationships writes relationships and merges the report.
func (ing *Ingester) writeRelationships(
	ctx context.Context,
	report *IngestReport,
	rels []ExtractedRelationship,
	nodeMap map[string]string,
) error {
	wr, err := ing.writer.WriteRelationships(ctx, rels, nodeMap)
	if err != nil {
		return fmt.Errorf("writing relationships: %w", err)
	}

	report.CreatedEdges = wr.CreatedEdges
	report.SkippedEdges = wr.SkippedEdges
	report.UnknownRelations = len(wr.UnknownRelations)

	return nil
}

// writeFacts writes facts to the graph, logging errors to the report.
func (ing *Ingester) writeFacts(
	ctx context.Context,
	facts []ExtractedFact,
	nodeMap map[string]string,
	report *IngestReport,
) error {
	if err := ing.writer.WriteFacts(ctx, facts, nodeMap); err != nil {
		report.Errors = append(report.Errors,
			fmt.Sprintf("writing facts: %v", err))
	}

	return nil
}

func (ing *Ingester) writeEpisodic(
	ctx context.Context,
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
	nodeMap map[string]string,
	report *IngestReport,
) error {
	createdEpisodes, createdEvents, err := ing.writer.WriteEpisodic(ctx, entities, rels, nodeMap)
	if err != nil {
		report.Errors = append(report.Errors,
			fmt.Sprintf("writing episodic records: %v", err))
		return nil
	}

	report.CreatedEpisodes += createdEpisodes
	report.CreatedEvents += createdEvents
	return nil
}
