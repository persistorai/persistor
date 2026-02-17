package client

import "context"

// BulkService handles batch operations.
type BulkService struct {
	c *Client
}

// bulkNodesResponse wraps the response from bulk node operations.
type bulkNodesResponse struct {
	Upserted int    `json:"upserted"`
	Nodes    []Node `json:"nodes"`
}

// bulkEdgesResponse wraps the response from bulk edge operations.
type bulkEdgesResponse struct {
	Upserted int    `json:"upserted"`
	Edges    []Edge `json:"edges"`
}

// UpsertNodes creates or updates nodes in bulk (max 1000).
// Returns the upserted nodes.
func (s *BulkService) UpsertNodes(ctx context.Context, nodes []CreateNodeRequest) ([]Node, error) {
	var resp bulkNodesResponse
	if err := s.c.post(ctx, "/api/v1/bulk/nodes", nodes, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// UpsertEdges creates or updates edges in bulk (max 1000).
// Returns the upserted edges.
func (s *BulkService) UpsertEdges(ctx context.Context, edges []CreateEdgeRequest) ([]Edge, error) {
	var resp bulkEdgesResponse
	if err := s.c.post(ctx, "/api/v1/bulk/edges", edges, &resp); err != nil {
		return nil, err
	}
	return resp.Edges, nil
}
