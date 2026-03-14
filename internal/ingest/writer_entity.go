package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/persistorai/persistor/client"
)

// writeEntity searches for an existing node by name, creates or updates it.
func (w *Writer) writeEntity(
	ctx context.Context,
	ent ExtractedEntity,
) (string, entityAction, error) {
	existing, err := w.findByName(ctx, ent.Name)
	if err != nil {
		return "", 0, fmt.Errorf("searching for entity %q: %w", ent.Name, err)
	}

	if existing != nil {
		id, err := w.updateEntity(ctx, existing, ent)
		if err != nil {
			return "", 0, err
		}
		return id, actionUpdated, nil
	}

	id, err := w.createEntity(ctx, ent)
	if err != nil {
		return "", 0, err
	}
	return id, actionCreated, nil
}

// findByName looks up a node by exact label match first, then falls back to
// fuzzy matching via full-text search if no exact match is found.
func (w *Writer) findByName(ctx context.Context, name string) (*client.Node, error) {
	node, err := w.graph.GetNodeByLabel(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("looking up node by label %q: %w", name, err)
	}
	if node != nil {
		return node, nil
	}

	// Fall back to fuzzy search
	return w.findByNameFuzzy(ctx, name)
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
