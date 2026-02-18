package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// EdgeStore is the data-access interface EdgeService depends on.
// It reuses domain.EdgeService since the method sets are identical, avoiding duplication.
type EdgeStore = domain.EdgeService

// Compile-time check: *EdgeService must satisfy domain.EdgeService.
var _ domain.EdgeService = (*EdgeService)(nil)

// EdgeService wraps EdgeStore with audit logging for mutations.
type EdgeService struct {
	store       EdgeStore
	auditWorker AuditEnqueuer
	log         *logrus.Logger
}

// NewEdgeService creates an EdgeService.
func NewEdgeService(store EdgeStore, auditWorker AuditEnqueuer, log *logrus.Logger) *EdgeService {
	return &EdgeService{store: store, auditWorker: auditWorker, log: log}
}

// auditAsync enqueues an audit entry via the AuditWorker (best-effort, non-blocking).
func (s *EdgeService) auditAsync(tenantID, action, entityType, entityID string, detail map[string]any) {
	if s.auditWorker == nil {
		return
	}
	s.auditWorker.Enqueue(&AuditJob{
		TenantID:   tenantID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Detail:     detail,
	})
}

// ListEdges returns a paginated list of edges (pass-through).
func (s *EdgeService) ListEdges(
	ctx context.Context, tenantID string, source, target, relation string, limit, offset int,
) ([]models.Edge, bool, error) {
	return s.store.ListEdges(ctx, tenantID, source, target, relation, limit, offset)
}

// CreateEdge creates an edge and records an audit entry.
func (s *EdgeService) CreateEdge(
	ctx context.Context, tenantID string, req models.CreateEdgeRequest,
) (*models.Edge, error) {
	edge, err := s.store.CreateEdge(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	s.auditAsync(tenantID, "edge.create", "edge", edge.Source+"/"+edge.Target+"/"+edge.Relation,
		map[string]any{"source": edge.Source, "target": edge.Target, "relation": edge.Relation})

	return edge, nil
}

// UpdateEdge updates an edge and records an audit entry.
func (s *EdgeService) UpdateEdge(
	ctx context.Context, tenantID string, source, target, relation string, req models.UpdateEdgeRequest,
) (*models.Edge, error) {
	edge, err := s.store.UpdateEdge(ctx, tenantID, source, target, relation, req)
	if err != nil {
		return nil, err
	}

	s.auditAsync(tenantID, "edge.update", "edge", source+"/"+target+"/"+relation,
		map[string]any{"source": source, "target": target, "relation": relation})

	return edge, nil
}

// PatchEdgeProperties partially updates edge properties (merge semantics).
func (s *EdgeService) PatchEdgeProperties(
	ctx context.Context, tenantID string, source, target, relation string, req models.PatchPropertiesRequest,
) (*models.Edge, error) {
	edge, err := s.store.PatchEdgeProperties(ctx, tenantID, source, target, relation, req)
	if err != nil {
		return nil, err
	}

	s.auditAsync(tenantID, "edge.patch_properties", "edge", source+"/"+target+"/"+relation, nil)

	return edge, nil
}

// DeleteEdge removes an edge and records an audit entry.
func (s *EdgeService) DeleteEdge(ctx context.Context, tenantID, source, target, relation string) error {
	err := s.store.DeleteEdge(ctx, tenantID, source, target, relation)
	if err == nil {
		s.auditAsync(tenantID, "edge.delete", "edge", source+"/"+target+"/"+relation,
			map[string]any{"source": source, "target": target, "relation": relation})
	}
	return err
}
