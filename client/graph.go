package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// GraphService handles graph traversal operations.
type GraphService struct {
	c *Client
}

// Neighbors returns nodes and edges directly connected to a node.
func (s *GraphService) Neighbors(ctx context.Context, id string, limit int) (*NeighborResult, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var resp NeighborResult
	if err := s.c.get(ctx, "/api/v1/graph/neighbors/"+url.PathEscape(id), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Traverse performs a BFS traversal from a node up to maxHops deep.
func (s *GraphService) Traverse(ctx context.Context, id string, maxHops int) (*TraverseResult, error) {
	params := url.Values{}
	if maxHops > 0 {
		params.Set("hops", strconv.Itoa(maxHops))
	}
	var resp TraverseResult
	if err := s.c.get(ctx, "/api/v1/graph/traverse/"+url.PathEscape(id), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Context returns a node with its immediate neighborhood.
func (s *GraphService) Context(ctx context.Context, id string) (*ContextResult, error) {
	var resp ContextResult
	if err := s.c.get(ctx, "/api/v1/graph/context/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ShortestPath finds the shortest path between two nodes.
func (s *GraphService) ShortestPath(ctx context.Context, fromID, toID string) ([]Node, error) {
	path := fmt.Sprintf("/api/v1/graph/path/%s/%s", url.PathEscape(fromID), url.PathEscape(toID))
	var resp struct {
		Path []Node `json:"path"`
	}
	if err := s.c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Path, nil
}
