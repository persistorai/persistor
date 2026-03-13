package service

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// UnknownRelationStore is the data-access interface for unknown relation operations.
type UnknownRelationStore interface {
	LogUnknownRelation(ctx context.Context, tenantID, relationType, sourceName, targetName, sourceText string) error
	ListUnknownRelations(ctx context.Context, tenantID string, opts models.UnknownRelationListOpts) ([]models.UnknownRelation, error)
	ResolveUnknownRelation(ctx context.Context, tenantID, id, canonicalType string) error
}

// UnknownRelationService provides business logic for unknown relation tracking.
type UnknownRelationService struct {
	store UnknownRelationStore
	log   *logrus.Logger
}

// NewUnknownRelationService creates an UnknownRelationService.
func NewUnknownRelationService(store UnknownRelationStore, log *logrus.Logger) *UnknownRelationService {
	return &UnknownRelationService{store: store, log: log}
}

// LogUnknownRelation records an unknown relation type for later review.
func (s *UnknownRelationService) LogUnknownRelation(
	ctx context.Context,
	tenantID, relationType, sourceName, targetName, sourceText string,
) error {
	if err := s.store.LogUnknownRelation(ctx, tenantID, relationType, sourceName, targetName, sourceText); err != nil {
		return fmt.Errorf("logging unknown relation: %w", err)
	}

	return nil
}

// ListUnknownRelations returns unknown relations matching the given options.
func (s *UnknownRelationService) ListUnknownRelations(
	ctx context.Context,
	tenantID string,
	opts models.UnknownRelationListOpts,
) ([]models.UnknownRelation, error) {
	results, err := s.store.ListUnknownRelations(ctx, tenantID, opts)
	if err != nil {
		return nil, fmt.Errorf("listing unknown relations: %w", err)
	}

	return results, nil
}

// ResolveUnknownRelation marks an unknown relation as resolved with a canonical type.
func (s *UnknownRelationService) ResolveUnknownRelation(
	ctx context.Context,
	tenantID, id, canonicalType string,
) error {
	if err := s.store.ResolveUnknownRelation(ctx, tenantID, id, canonicalType); err != nil {
		return fmt.Errorf("resolving unknown relation: %w", err)
	}

	return nil
}
