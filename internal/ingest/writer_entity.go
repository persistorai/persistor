package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/persistorai/persistor/client"
)

// writeEntity searches for an existing node by name, creates or updates it.
func (w *Writer) writeEntity(
	ctx context.Context,
	ent ExtractedEntity,
) (string, entityAction, error) {
	resolution, err := w.resolveEntity(ctx, ent.Name, ent.Type)
	if err != nil {
		return "", 0, fmt.Errorf("resolving entity %q: %w", ent.Name, err)
	}

	if resolution.Status == resolutionMatched && resolution.Match != nil && resolution.Match.Node != nil {
		id, err := w.updateEntity(ctx, resolution.Match.Node, ent)
		if err != nil {
			return "", 0, err
		}
		return id, actionUpdated, nil
	}

	if resolution.Status == resolutionAmbiguous {
		logEntityResolution(ent.Name, resolution)
	}

	id, err := w.createEntity(ctx, ent)
	if err != nil {
		return "", 0, err
	}
	return id, actionCreated, nil
}

func logEntityResolution(name string, resolution *entityResolution) {
	topCandidates := make([]any, 0, len(resolution.Candidates))
	for _, candidate := range resolution.Candidates {
		topCandidates = append(topCandidates, map[string]any{
			"id":         candidate.Node.ID,
			"label":      candidate.Node.Label,
			"type":       candidate.Node.Type,
			"method":     candidate.Method,
			"confidence": candidate.Confidence,
		})
	}
	slog.Info("ambiguous entity resolution",
		"query", name,
		"expected_type", resolution.ExpectedType,
		"candidates", topCandidates,
	)
}

// createEntity creates a new node for the extracted entity.
func (w *Writer) createEntity(ctx context.Context, ent ExtractedEntity) (string, error) {
	props := buildCreateProps(ent.Properties, w.source)

	req := &client.CreateNodeRequest{
		Type:       ent.Type,
		Label:      ent.Name,
		Properties: props,
	}

	node, err := w.graph.CreateNode(ctx, req)
	if err != nil {
		return "", fmt.Errorf("creating node %q: %w", ent.Name, err)
	}

	return node.ID, nil
}

// buildCreateProps merges entity properties with ingest metadata.
func buildCreateProps(props map[string]any, source string) map[string]any {
	result := make(map[string]any, len(props)+2)
	for k, v := range props {
		result[k] = v
	}
	result["_ingested_from"] = source
	result["_ingested_at"] = time.Now().UTC().Format(time.RFC3339)
	return result
}

// updateEntity merges properties onto an existing node.
func (w *Writer) updateEntity(
	ctx context.Context,
	existing *client.Node,
	ent ExtractedEntity,
) (string, error) {
	merged := mergeProperties(existing.Properties, ent.Properties, w.source)

	_, err := w.graph.PatchNodeProperties(ctx, existing.ID, merged)
	if err != nil {
		return "", fmt.Errorf("updating node %q: %w", ent.Name, err)
	}

	return existing.ID, nil
}
