package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// HistoryStore is the data-access interface HistoryService depends on.
// It reuses domain.HistoryService since the method sets are identical, avoiding duplication.
type HistoryStore = domain.HistoryService

// Compile-time check: *HistoryService must satisfy domain.HistoryService.
var _ domain.HistoryService = (*HistoryService)(nil)

// HistoryService wraps HistoryStore with context-aware logging.
type HistoryService struct {
	store HistoryStore
	log   *logrus.Logger
}

// NewHistoryService creates a HistoryService.
func NewHistoryService(store HistoryStore, log *logrus.Logger) *HistoryService {
	return &HistoryService{store: store, log: log}
}

// GetPropertyHistory returns property change history for a node with optional key filter.
func (s *HistoryService) GetPropertyHistory(
	ctx context.Context, tenantID, nodeID, propertyKey string, limit, offset int,
) ([]models.PropertyChange, bool, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id":    tenantID,
		"node_id":      nodeID,
		"property_key": propertyKey,
		"limit":        limit,
		"offset":       offset,
	}).Debug("history.get_property_history")

	return s.store.GetPropertyHistory(ctx, tenantID, nodeID, propertyKey, limit, offset)
}
