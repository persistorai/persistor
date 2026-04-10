package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

// AdminStore is the data-access interface AdminService depends on.
type AdminStore interface {
	ListNodesWithoutEmbeddings(ctx context.Context, tenantID string, limit int) ([]models.NodeSummary, error)
	ListNodesForReprocess(ctx context.Context, tenantID string, limit int) ([]store.ReprocessableNode, error)
	CountNodesForReprocess(ctx context.Context, tenantID string) (remainingSearchText, remainingEmbeddings, remainingTotal int, err error)
	UpdateNodeSearchText(ctx context.Context, tenantID, nodeID, searchText string) error
}

// Compile-time check: *AdminService must satisfy domain.AdminService.
var _ domain.AdminService = (*AdminService)(nil)

// AdminService wraps AdminStore with context-aware logging.
type AdminService struct {
	store       AdminStore
	embedWorker EmbedEnqueuer
	log         *logrus.Logger
}

// NewAdminService creates an AdminService.
func NewAdminService(store AdminStore, embedWorker EmbedEnqueuer, log *logrus.Logger) *AdminService {
	return &AdminService{store: store, embedWorker: embedWorker, log: log}
}

// ListNodesWithoutEmbeddings returns nodes with a NULL embedding vector, up to limit.
func (s *AdminService) ListNodesWithoutEmbeddings(ctx context.Context, tenantID string, limit int) ([]models.NodeSummary, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"limit":     limit,
	}).Debug("admin.list_nodes_without_embeddings")

	return s.store.ListNodesWithoutEmbeddings(ctx, tenantID, limit)
}
