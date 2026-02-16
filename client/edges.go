package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// EdgeService handles edge CRUD operations.
type EdgeService struct {
	c *Client
}

// edgeListResponse wraps the paginated edge list response.
type edgeListResponse struct {
	Edges   []Edge `json:"edges"`
	HasMore bool   `json:"has_more"`
}

// List returns edges with optional filtering and pagination.
func (s *EdgeService) List(ctx context.Context, opts *EdgeListOptions) ([]Edge, bool, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Source != "" {
			params.Set("source", opts.Source)
		}
		if opts.Target != "" {
			params.Set("target", opts.Target)
		}
		if opts.Relation != "" {
			params.Set("relation", opts.Relation)
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
	}
	var resp edgeListResponse
	if err := s.c.get(ctx, "/api/v1/edges", params, &resp); err != nil {
		return nil, false, err
	}
	return resp.Edges, resp.HasMore, nil
}

// Create creates a new edge.
func (s *EdgeService) Create(ctx context.Context, req *CreateEdgeRequest) (*Edge, error) {
	var edge Edge
	if err := s.c.post(ctx, "/api/v1/edges", req, &edge); err != nil {
		return nil, err
	}
	return &edge, nil
}

// Update updates an existing edge by source/target/relation.
func (s *EdgeService) Update(ctx context.Context, source, target, relation string, req *UpdateEdgeRequest) (*Edge, error) {
	path := fmt.Sprintf("/api/v1/edges/%s/%s/%s",
		url.PathEscape(source), url.PathEscape(target), url.PathEscape(relation))
	var edge Edge
	if err := s.c.put(ctx, path, req, &edge); err != nil {
		return nil, err
	}
	return &edge, nil
}

// Delete removes an edge by source/target/relation.
func (s *EdgeService) Delete(ctx context.Context, source, target, relation string) error {
	path := fmt.Sprintf("/api/v1/edges/%s/%s/%s",
		url.PathEscape(source), url.PathEscape(target), url.PathEscape(relation))
	return s.c.del(ctx, path, nil, nil)
}
