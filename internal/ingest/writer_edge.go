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

	for i := range relationships {
		w.writeRelationship(ctx, &relationships[i], nodeMap, report)
	}

	return report, nil
}

// writeRelationship processes a single relationship.
func (w *Writer) writeRelationship(
	ctx context.Context,
	rel *ExtractedRelationship,
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
		report.UnknownRelations = append(report.UnknownRelations, *rel)
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

// createEdge creates or updates an edge via the graph client.
// On 409 conflict (edge exists), falls back to updating the existing edge.
func (w *Writer) createEdge(
	ctx context.Context,
	sourceID, targetID string,
	rel *ExtractedRelationship,
) error {
	req := &client.CreateEdgeRequest{
		Source:   sourceID,
		Target:   targetID,
		Relation: rel.Relation,
		Properties: map[string]any{
			"_confidence":    rel.Confidence,
			"_ingested_from": w.source,
		},
		DateStart: rel.DateStart,
		DateEnd:   rel.DateEnd,
		IsCurrent: rel.IsCurrent,
	}

	_, err := w.graph.CreateEdge(ctx, req)
	if err == nil {
		return nil
	}

	// If edge already exists (409), update it with new data
	if !strings.Contains(err.Error(), "409") {
		return fmt.Errorf("creating edge %s->%s: %w", rel.Source, rel.Target, err)
	}

	return w.updateExistingEdge(ctx, sourceID, targetID, rel)
}

// updateExistingEdge updates an existing edge with new temporal and property data.
func (w *Writer) updateExistingEdge(
	ctx context.Context,
	sourceID, targetID string,
	rel *ExtractedRelationship,
) error {
	updateReq := &client.UpdateEdgeRequest{
		Properties: map[string]any{
			"_confidence":    rel.Confidence,
			"_ingested_from": w.source,
		},
		DateStart: rel.DateStart,
		DateEnd:   rel.DateEnd,
		IsCurrent: rel.IsCurrent,
	}

	_, err := w.graph.UpdateEdge(ctx, sourceID, targetID, rel.Relation, updateReq)
	if err != nil {
		return fmt.Errorf("updating edge %s->%s: %w", rel.Source, rel.Target, err)
	}

	return nil
}

// resolveEntityInKG resolves an entity name against existing nodes.
// Ambiguous candidates are treated as unresolved to avoid silent merges.
func (w *Writer) resolveEntityInKG(
	ctx context.Context,
	name string,
	nodeMap map[string]string,
) (string, bool) {
	resolution, err := w.resolveEntity(ctx, name, "")
	if err != nil || resolution.Status != resolutionMatched || resolution.Match == nil || resolution.Match.Node == nil {
		if err == nil && resolution != nil && resolution.Status == resolutionAmbiguous {
			logEntityResolution(name, resolution)
		}
		return "", false
	}
	// Cache for future lookups in this ingest run
	nodeMap[strings.ToLower(name)] = resolution.Match.Node.ID
	return resolution.Match.Node.ID, true
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

	update := models.FactUpdate{Value: fact.Value, Source: w.source, Confidence: fact.Confidence}
	if fact.Timestamp != nil {
		update.Timestamp = *fact.Timestamp
	}
	props := map[string]any{
		models.FactUpdatesProperty: map[string]models.FactUpdate{
			fact.Property: update,
		},
	}

	_, err := w.graph.PatchNodeProperties(ctx, nodeID, props)
	if err != nil {
		return fmt.Errorf("patching fact on %q: %w", fact.Subject, err)
	}

	return nil
}
