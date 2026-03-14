package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/persistorai/persistor/client"
	"github.com/persistorai/persistor/internal/models"
)

// WriteRelationships creates edges for canonical relationships, collects unknowns.
func (w *Writer) WriteRelationships(
	ctx context.Context,
	relationships []ExtractedRelationship,
	nodeMap map[string]string,
) (*WriteReport, error) {
	report := &WriteReport{}

	for _, rel := range relationships {
		w.writeRelationship(ctx, rel, nodeMap, report)
	}

	return report, nil
}

// writeRelationship processes a single relationship.
func (w *Writer) writeRelationship(
	ctx context.Context,
	rel ExtractedRelationship,
	nodeMap map[string]string,
	report *WriteReport,
) {
	sourceID, sourceOK := nodeMap[strings.ToLower(rel.Source)]
	targetID, targetOK := nodeMap[strings.ToLower(rel.Target)]

	// If entity not in current nodeMap, try to find it in the KG
	if !sourceOK {
		sourceID, sourceOK = w.resolveEntityInKG(ctx, rel.Source, nodeMap)
	}
	if !targetOK {
		targetID, targetOK = w.resolveEntityInKG(ctx, rel.Target, nodeMap)
	}

	if !sourceOK || !targetOK {
		report.SkippedEdges++
		return
	}

	if !models.IsCanonicalRelation(rel.Relation) {
		report.UnknownRelations = append(report.UnknownRelations, rel)
		report.SkippedEdges++
		return
	}

	err := w.createEdge(ctx, sourceID, targetID, rel)
	if err != nil {
		report.SkippedEdges++
		return
	}

	report.CreatedEdges++
}

// createEdge creates a single edge via the graph client.
func (w *Writer) createEdge(
	ctx context.Context,
	sourceID, targetID string,
	rel ExtractedRelationship,
) error {
	req := &client.CreateEdgeRequest{
		Source:   sourceID,
		Target:   targetID,
		Relation: rel.Relation,
		Properties: map[string]any{
			"_confidence":    rel.Confidence,
			"_ingested_from": w.source,
		},
	}

	_, err := w.graph.CreateEdge(ctx, req)
	if err != nil {
		return fmt.Errorf("creating edge %s->%s: %w", rel.Source, rel.Target, err)
	}

	return nil
}

// resolveEntityInKG looks up an entity name in the KG via exact label match.
// If found, it caches the result in nodeMap for future lookups.
func (w *Writer) resolveEntityInKG(
	ctx context.Context,
	name string,
	nodeMap map[string]string,
) (string, bool) {
	existing, err := w.findByName(ctx, name)
	if err != nil || existing == nil {
		return "", false
	}
	// Cache for future lookups in this ingest run
	nodeMap[strings.ToLower(name)] = existing.ID
	return existing.ID, true
}

// WriteFacts patches extracted facts onto existing nodes.
func (w *Writer) WriteFacts(
	ctx context.Context,
	facts []ExtractedFact,
	nodeMap map[string]string,
) error {
	for _, fact := range facts {
		if err := w.writeFact(ctx, fact, nodeMap); err != nil {
			continue
		}
	}

	return nil
}

// writeFact patches a single fact onto its subject node.
func (w *Writer) writeFact(
	ctx context.Context,
	fact ExtractedFact,
	nodeMap map[string]string,
) error {
	nodeID, ok := nodeMap[strings.ToLower(fact.Subject)]
	if !ok {
		// Try to find in KG
		nodeID, ok = w.resolveEntityInKG(ctx, fact.Subject, nodeMap)
		if !ok {
			return fmt.Errorf("subject %q not in node map or KG", fact.Subject)
		}
	}

	props := map[string]any{fact.Property: fact.Value}

	_, err := w.graph.PatchNodeProperties(ctx, nodeID, props)
	if err != nil {
		return fmt.Errorf("patching fact on %q: %w", fact.Subject, err)
	}

	return nil
}
