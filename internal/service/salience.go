package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// SalienceStore is the data-access interface SalienceService depends on.
// It reuses domain.SalienceService since the method sets are identical, avoiding duplication.
type SalienceStore = domain.SalienceService

// Compile-time check: *SalienceService must satisfy domain.SalienceService.
var _ domain.SalienceService = (*SalienceService)(nil)

// SalienceService wraps SalienceStore with audit logging for mutations.
type SalienceService struct {
	store       SalienceStore
	auditWorker AuditEnqueuer
	log         *logrus.Logger
}

// NewSalienceService creates a SalienceService.
func NewSalienceService(store SalienceStore, auditWorker AuditEnqueuer, log *logrus.Logger) *SalienceService {
	return &SalienceService{store: store, auditWorker: auditWorker, log: log}
}

// BoostNode sets user_boosted to TRUE, recalculates salience, and records an audit entry.
func (s *SalienceService) BoostNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	node, err := s.store.BoostNode(ctx, tenantID, nodeID)
	if err != nil {
		return nil, err
	}

	auditAsync(s.auditWorker, tenantID, "salience.boost", "node", nodeID, nil)

	return node, nil
}

// SupersedeNode marks oldID as superseded by newID and records an audit entry.
func (s *SalienceService) SupersedeNode(ctx context.Context, tenantID, oldID, newID string) error {
	if err := s.store.SupersedeNode(ctx, tenantID, oldID, newID); err != nil {
		return err
	}

	auditAsync(s.auditWorker, tenantID, "salience.supersede", "node", oldID, map[string]any{"new_id": newID})

	return nil
}

// RecalculateSalience recomputes salience scores for all tenant nodes and records an audit entry.
func (s *SalienceService) RecalculateSalience(ctx context.Context, tenantID string) (int, error) {
	count, err := s.store.RecalculateSalience(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	auditAsync(s.auditWorker, tenantID, "salience.recalculate", "node", "", map[string]any{"updated": count})

	return count, nil
}
