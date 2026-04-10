package client

import (
	"context"
	"net/url"
	"strconv"

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

// RunMaintenance triggers an explicit refresh/reprocess maintenance pass.
func (s *AdminService) RunMaintenance(ctx context.Context, req models.MaintenanceRunRequest) (*models.MaintenanceRunResult, error) {
	var resp models.MaintenanceRunResult
	if err := s.c.post(ctx, "/api/v1/admin/maintenance/run", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListMergeSuggestions returns explainable duplicate candidates for manual review.
func (s *AdminService) ListMergeSuggestions(ctx context.Context, opts models.MergeSuggestionListOpts) ([]models.MergeSuggestion, error) {
	query := make(url.Values)
	if opts.Type != "" {
		query.Set("type", opts.Type)
	}
	if opts.Limit > 0 {
		query.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.MinScore > 0 {
		query.Set("min_score", strconv.FormatFloat(opts.MinScore, 'f', -1, 64))
	}
	var resp struct {
		Suggestions []models.MergeSuggestion `json:"suggestions"`
	}
	if err := s.c.get(ctx, "/api/v1/admin/merge-suggestions", query, &resp); err != nil {
		return nil, err
	}
	return resp.Suggestions, nil
}

// HistoryService handles property history operations.
// Note: History is accessed via NodeService.History() for convenience,
// but this service exists for direct access if needed.
type HistoryService struct {
	c *Client
}
