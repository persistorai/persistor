package client

import "context"

// BulkService handles batch operations.
type BulkService struct {
	c *Client
}

// bulkResponse wraps the count returned by bulk operations.
type bulkResponse struct {
	Upserted int `json:"upserted"`
}

// UpsertNodes creates or updates nodes in bulk (max 1000).
func (s *BulkService) UpsertNodes(ctx context.Context, nodes []CreateNodeRequest) (int, error) {
	var resp bulkResponse
	if err := s.c.post(ctx, "/api/v1/bulk/nodes", nodes, &resp); err != nil {
		return 0, err
	}
	return resp.Upserted, nil
}

// UpsertEdges creates or updates edges in bulk (max 1000).
func (s *BulkService) UpsertEdges(ctx context.Context, edges []CreateEdgeRequest) (int, error) {
	var resp bulkResponse
	if err := s.c.post(ctx, "/api/v1/bulk/edges", edges, &resp); err != nil {
		return 0, err
	}
	return resp.Upserted, nil
}
