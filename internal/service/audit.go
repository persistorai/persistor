package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// AuditQueryStore is the data-access interface AuditService depends on.
// It reuses domain.AuditService since the method sets are identical, avoiding duplication.
type AuditQueryStore = domain.AuditService

// Compile-time check: *AuditService must satisfy domain.AuditService.
var _ domain.AuditService = (*AuditService)(nil)

// AuditService wraps AuditQueryStore with logging for destructive operations.
type AuditService struct {
	store AuditQueryStore
	log   *logrus.Logger
}

// NewAuditService creates an AuditService.
func NewAuditService(store AuditQueryStore, log *logrus.Logger) *AuditService {
	return &AuditService{store: store, log: log}
}

// RecordAudit inserts an audit log entry (pass-through to store).
func (s *AuditService) RecordAudit(
	ctx context.Context, tenantID, action, entityType, entityID, actor string, detail map[string]any,
) error {
	return s.store.RecordAudit(ctx, tenantID, action, entityType, entityID, actor, detail)
}

// QueryAudit returns audit entries matching the given filters (pass-through).
func (s *AuditService) QueryAudit(
	ctx context.Context, tenantID string, opts models.AuditQueryOpts,
) ([]models.AuditEntry, bool, error) {
	return s.store.QueryAudit(ctx, tenantID, opts)
}

// PurgeOldEntries deletes audit entries older than retentionDays and logs the result.
func (s *AuditService) PurgeOldEntries(ctx context.Context, tenantID string, retentionDays int) (int, error) {
	deleted, err := s.store.PurgeOldEntries(ctx, tenantID, retentionDays)
	if err != nil {
		return 0, err
	}

	s.log.WithFields(logrus.Fields{
		"tenant_id":      tenantID,
		"retention_days": retentionDays,
		"deleted":        deleted,
	}).Info("audit.purge")

	return deleted, nil
}
