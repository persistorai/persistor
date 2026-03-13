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

// SearchNodes performs a full-text search for nodes.
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

// CreateEdge creates a new edge via the API.
func (p *persistorClient) CreateEdge(
	ctx context.Context,
	req *client.CreateEdgeRequest,
) (*client.Edge, error) {
	return p.c.Edges.Create(ctx, req)
}
