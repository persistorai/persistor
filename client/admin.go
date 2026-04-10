package client

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

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

// ReprocessNodes rewrites search text and/or queues embeddings for existing nodes.
func (s *AdminService) ReprocessNodes(ctx context.Context, req models.ReprocessNodesRequest) (*models.ReprocessNodesResult, error) {
	var resp models.ReprocessNodesResult
	if err := s.c.post(ctx, "/api/v1/admin/reprocess-nodes", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// HistoryService handles property history operations.
// Note: History is accessed via NodeService.History() for convenience,
// but this service exists for direct access if needed.
type HistoryService struct {
	c *Client
}
