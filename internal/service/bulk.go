package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// BulkStore defines the data access methods BulkService depends on.
type BulkStore interface {
	BulkUpsertNodes(ctx context.Context, tenantID string, nodes []models.CreateNodeRequest) (int, error)
	BulkUpsertEdges(ctx context.Context, tenantID string, edges []models.CreateEdgeRequest) (int, error)
}

// BulkService wraps BulkStore with embedding enqueue logic for bulk node upserts.
type BulkService struct {
	store       BulkStore
	embedWorker EmbedEnqueuer
	auditWorker *AuditWorker
	log         *logrus.Logger
}

// NewBulkService creates a BulkService.
func NewBulkService(store BulkStore, embedWorker EmbedEnqueuer, auditWorker *AuditWorker, log *logrus.Logger) *BulkService {
	return &BulkService{store: store, embedWorker: embedWorker, auditWorker: auditWorker, log: log}
}

// BulkUpsertNodes upserts nodes and enqueues embedding jobs for each.
func (s *BulkService) BulkUpsertNodes(
	ctx context.Context, tenantID string, nodes []models.CreateNodeRequest,
) (int, error) {
	count, err := s.store.BulkUpsertNodes(ctx, tenantID, nodes)
	if err != nil {
		return 0, err
	}

	if s.embedWorker != nil {
		for _, req := range nodes {
			s.embedWorker.Enqueue(EmbedJob{
				TenantID: tenantID,
				NodeID:   req.ID,
				Text:     req.Type + ":" + req.Label,
			})
		}
	}

	if s.auditWorker != nil {
		s.auditWorker.Enqueue(&AuditJob{
			TenantID: tenantID, Action: "bulk.nodes", EntityType: "node",
			Detail: map[string]any{"count": count},
		})
	}

	return count, nil
}

// BulkUpsertEdges upserts edges (pass-through).
func (s *BulkService) BulkUpsertEdges(
	ctx context.Context, tenantID string, edges []models.CreateEdgeRequest,
) (int, error) {
	count, err := s.store.BulkUpsertEdges(ctx, tenantID, edges)
	if err != nil {
		return 0, err
	}

	if s.auditWorker != nil {
		s.auditWorker.Enqueue(&AuditJob{
			TenantID: tenantID, Action: "bulk.edges", EntityType: "edge",
			Detail: map[string]any{"count": count},
		})
	}

	return count, nil
}
