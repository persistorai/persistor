package ingest

import (
	"context"
	"fmt"
	"strings"
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

// findByName searches for a node matching the entity name (case-insensitive).
func (w *Writer) findByName(ctx context.Context, name string) (*client.Node, error) {
	nodes, err := w.graph.SearchNodes(ctx, name, 5)
	if err != nil {
		return nil, fmt.Errorf("searching nodes: %w", err)
	}

	for i := range nodes {
		if strings.EqualFold(nodes[i].Label, name) {
			return &nodes[i], nil
		}
	}

	return nil, nil
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
