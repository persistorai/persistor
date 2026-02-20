package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// AdminStore is the data-access interface AdminService depends on.
// It reuses domain.AdminService since the method sets are identical, avoiding duplication.
type AdminStore = domain.AdminService

// Compile-time check: *AdminService must satisfy domain.AdminService.
var _ domain.AdminService = (*AdminService)(nil)

// AdminService wraps AdminStore with context-aware logging.
type AdminService struct {
	store AdminStore
	log   *logrus.Logger
}

// NewAdminService creates an AdminService.
func NewAdminService(store AdminStore, log *logrus.Logger) *AdminService {
	return &AdminService{store: store, log: log}
}

// ListNodesWithoutEmbeddings returns nodes with a NULL embedding vector, up to limit.
func (s *AdminService) ListNodesWithoutEmbeddings(ctx context.Context, tenantID string, limit int) ([]models.NodeSummary, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"limit":     limit,
	}).Debug("admin.list_nodes_without_embeddings")

	return s.store.ListNodesWithoutEmbeddings(ctx, tenantID, limit)
}
