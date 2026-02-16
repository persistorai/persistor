package client

import "context"

// AdminService handles administrative operations.
type AdminService struct {
	c *Client
}

// BackfillEmbeddings queues embedding generation for nodes without embeddings.
// Returns the number of nodes queued.
func (s *AdminService) BackfillEmbeddings(ctx context.Context) (int, error) {
	var resp struct {
		Queued int `json:"queued"`
	}
	if err := s.c.post(ctx, "/api/v1/admin/backfill-embeddings", nil, &resp); err != nil {
		return 0, err
	}
	return resp.Queued, nil
}

// HistoryService handles property history operations.
// Note: History is accessed via NodeService.History() for convenience,
// but this service exists for direct access if needed.
type HistoryService struct {
	c *Client
}
