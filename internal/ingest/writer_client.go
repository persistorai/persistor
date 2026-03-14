package ingest

import (
	"context"

	"github.com/persistorai/persistor/client"
)

// persistorClient wraps *client.Client to implement GraphClient.
type persistorClient struct {
	c *client.Client
}

// NewPersistorClient creates a GraphClient backed by the Persistor client SDK.
func NewPersistorClient(c *client.Client) GraphClient {
	return &persistorClient{c: c}
}

// GetNodeByLabel returns the node whose label matches exactly (case-insensitive),
// or nil if no match is found.
func (p *persistorClient) GetNodeByLabel(ctx context.Context, label string) (*client.Node, error) {
	return p.c.Nodes.GetByLabel(ctx, label)
}

// SearchNodes performs a full-text search returning up to limit nodes.
func (p *persistorClient) SearchNodes(
	ctx context.Context,
	query string,
	limit int,
) ([]client.Node, error) {
	return p.c.Search.FullText(ctx, query, &client.SearchOptions{Limit: limit})
}

// CreateNode creates a new node via the API.
func (p *persistorClient) CreateNode(
	ctx context.Context,
	req *client.CreateNodeRequest,
) (*client.Node, error) {
	return p.c.Nodes.Create(ctx, req)
}

// UpdateNode updates an existing node via the API.
func (p *persistorClient) UpdateNode(
	ctx context.Context,
	id string,
	req *client.UpdateNodeRequest,
) (*client.Node, error) {
	return p.c.Nodes.Update(ctx, id, req)
}

// PatchNodeProperties merges properties onto an existing node.
func (p *persistorClient) PatchNodeProperties(
	ctx context.Context,
	id string,
	properties map[string]any,
) (*client.Node, error) {
	return p.c.Nodes.PatchProperties(ctx, id, properties)
}

// UpdateEdge updates an existing edge via the API.
func (p *persistorClient) UpdateEdge(
	ctx context.Context,
	source, target, relation string,
	req *client.UpdateEdgeRequest,
) (*client.Edge, error) {
	return p.c.Edges.Update(ctx, source, target, relation, req)
}

// CreateEdge creates a new edge via the API.
func (p *persistorClient) CreateEdge(
	ctx context.Context,
	req *client.CreateEdgeRequest,
) (*client.Edge, error) {
	return p.c.Edges.Create(ctx, req)
}
