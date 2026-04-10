package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

func (s *NodeStore) buildUpdatedSearchText(
	ctx context.Context,
	tenantID string,
	req models.UpdateNodeRequest,
) (string, error) {
	base := &models.Node{}
	if req.Type != nil {
		base.Type = *req.Type
	}
	if req.Label != nil {
		base.Label = *req.Label
	}
	if req.Properties != nil {
		base.Properties = req.Properties
	}

	if req.Type != nil && req.Label != nil && req.Properties != nil {
		return models.BuildNodeSearchText(base), nil
	}

	current, err := s.GetNode(ctx, tenantID, "")
	_ = current
	_ = err
	return "", fmt.Errorf("buildUpdatedSearchText requires full node context")
}
