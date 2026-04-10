package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// AliasStore is the data-access interface AliasService depends on.
type AliasStore = domain.AliasService

// Compile-time check: *AliasService must satisfy domain.AliasService.
var _ domain.AliasService = (*AliasService)(nil)

// AliasService wraps AliasStore with business-logic entry points.
type AliasService struct {
	store AliasStore
	log   *logrus.Logger
}

// NewAliasService creates an AliasService.
func NewAliasService(store AliasStore, log *logrus.Logger) *AliasService {
	return &AliasService{store: store, log: log}
}

// CreateAlias creates a persisted alias.
func (s *AliasService) CreateAlias(ctx context.Context, tenantID string, req models.CreateAliasRequest) (*models.Alias, error) {
	return s.store.CreateAlias(ctx, tenantID, req)
}

// GetAlias returns a persisted alias by ID.
func (s *AliasService) GetAlias(ctx context.Context, tenantID, aliasID string) (*models.Alias, error) {
	return s.store.GetAlias(ctx, tenantID, aliasID)
}

// ListAliases lists persisted aliases with optional filters.
func (s *AliasService) ListAliases(ctx context.Context, tenantID string, opts models.AliasListOpts) ([]models.Alias, bool, error) {
	return s.store.ListAliases(ctx, tenantID, opts)
}

// DeleteAlias removes a persisted alias by ID.
func (s *AliasService) DeleteAlias(ctx context.Context, tenantID, aliasID string) error {
	return s.store.DeleteAlias(ctx, tenantID, aliasID)
}
