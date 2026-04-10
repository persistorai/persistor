package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

func (s *NodeStore) buildUpdatedSearchText(
	ctx context.Context,
	tenantID string,
	nodeID string,
	req models.UpdateNodeRequest,
) (string, error) {
	base, err := s.GetNode(ctx, tenantID, nodeID)
	if err != nil {
		return "", fmt.Errorf("loading current node for search text: %w", err)
	}

	if req.Type != nil {
		base.Type = *req.Type
	}
	if req.Label != nil {
		base.Label = *req.Label
	}
	if req.Properties != nil {
		base.Properties = req.Properties
	}

	return models.BuildNodeSearchText(base), nil
}
