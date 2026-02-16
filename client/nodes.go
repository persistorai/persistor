package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// NodeService handles node CRUD operations.
type NodeService struct {
	c *Client
}

// nodeListResponse wraps the paginated node list response.
type nodeListResponse struct {
	Nodes   []Node `json:"nodes"`
	HasMore bool   `json:"has_more"`
}

// List returns nodes with optional filtering and pagination.
func (s *NodeService) List(ctx context.Context, opts *NodeListOptions) ([]Node, bool, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Type != "" {
			params.Set("type", opts.Type)
		}
		if opts.MinSalience > 0 {
			params.Set("min_salience", strconv.FormatFloat(opts.MinSalience, 'f', -1, 64))
		}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
	}
	var resp nodeListResponse
	if err := s.c.get(ctx, "/api/v1/nodes", params, &resp); err != nil {
		return nil, false, err
	}
	return resp.Nodes, resp.HasMore, nil
}

// Get returns a single node by ID.
func (s *NodeService) Get(ctx context.Context, id string) (*Node, error) {
	var node Node
	if err := s.c.get(ctx, "/api/v1/nodes/"+url.PathEscape(id), nil, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// Create creates a new node.
func (s *NodeService) Create(ctx context.Context, req *CreateNodeRequest) (*Node, error) {
	var node Node
	if err := s.c.post(ctx, "/api/v1/nodes", req, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// Update updates an existing node by ID.
func (s *NodeService) Update(ctx context.Context, id string, req *UpdateNodeRequest) (*Node, error) {
	var node Node
	if err := s.c.put(ctx, "/api/v1/nodes/"+url.PathEscape(id), req, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// Delete removes a node by ID.
func (s *NodeService) Delete(ctx context.Context, id string) error {
	return s.c.del(ctx, "/api/v1/nodes/"+url.PathEscape(id), nil, nil)
}

// History returns property change history for a node.
func (s *NodeService) History(ctx context.Context, id string, property string, limit, offset int) ([]PropertyChange, bool, error) {
	params := url.Values{}
	if property != "" {
		params.Set("property", property)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	var resp struct {
		Changes []PropertyChange `json:"changes"`
		HasMore bool             `json:"has_more"`
	}
	if err := s.c.get(ctx, fmt.Sprintf("/api/v1/nodes/%s/history", url.PathEscape(id)), params, &resp); err != nil {
		return nil, false, err
	}
	return resp.Changes, resp.HasMore, nil
}
