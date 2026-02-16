package client

import (
	"context"
	"net/url"
	"strconv"
)

// SearchService handles search operations.
type SearchService struct {
	c *Client
}

// searchNodeResponse wraps search results returning nodes.
type searchNodeResponse struct {
	Nodes []Node `json:"nodes"`
	Total int    `json:"total"`
}

// searchScoredResponse wraps semantic search results with scores.
type searchScoredResponse struct {
	Nodes []ScoredNode `json:"nodes"`
	Total int          `json:"total"`
}

// FullText performs a full-text search.
func (s *SearchService) FullText(ctx context.Context, query string, opts *SearchOptions) ([]Node, error) {
	params := url.Values{"q": {query}}
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
	}
	var resp searchNodeResponse
	if err := s.c.get(ctx, "/api/v1/search", params, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// Semantic performs a semantic (vector) search.
func (s *SearchService) Semantic(ctx context.Context, query string, limit int) ([]ScoredNode, error) {
	params := url.Values{"q": {query}}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var resp searchScoredResponse
	if err := s.c.get(ctx, "/api/v1/search/semantic", params, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// Hybrid performs a hybrid (full-text + vector RRF fusion) search.
func (s *SearchService) Hybrid(ctx context.Context, query string, opts *SearchOptions) ([]Node, error) {
	params := url.Values{"q": {query}}
	if opts != nil && opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	var resp searchNodeResponse
	if err := s.c.get(ctx, "/api/v1/search/hybrid", params, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}
