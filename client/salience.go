package client

import (
	"context"
	"net/url"
)

// SalienceService handles salience scoring operations.
type SalienceService struct {
	c *Client
}

// Boost increases a node's salience score.
func (s *SalienceService) Boost(ctx context.Context, id string) (*Node, error) {
	var node Node
	if err := s.c.post(ctx, "/api/v1/salience/boost/"+url.PathEscape(id), nil, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// Supersede marks oldID as superseded by newID, transferring salience.
func (s *SalienceService) Supersede(ctx context.Context, oldID, newID string) error {
	req := SupersedeRequest{OldID: oldID, NewID: newID}
	return s.c.post(ctx, "/api/v1/salience/supersede", req, nil)
}

// Recalculate recalculates all salience scores. Returns the count of updated nodes.
func (s *SalienceService) Recalculate(ctx context.Context) (int, error) {
	var resp struct {
		Updated int `json:"updated"`
	}
	if err := s.c.post(ctx, "/api/v1/salience/recalc", nil, &resp); err != nil {
		return 0, err
	}
	return resp.Updated, nil
}
