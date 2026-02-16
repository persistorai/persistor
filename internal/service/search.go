package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// SearchStore defines the data access methods SearchService depends on.
type SearchStore interface {
	FullTextSearch(ctx context.Context, tenantID string, query string, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	SemanticSearch(ctx context.Context, tenantID string, embedding []float32, limit int) ([]models.ScoredNode, error)
	HybridSearch(ctx context.Context, tenantID string, query string, embedding []float32, limit int) ([]models.Node, error)
}

// Embedder generates vector embeddings from text.
type Embedder interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

// SearchService wraps SearchStore with embedding generation logic.
type SearchService struct {
	store    SearchStore
	embedder Embedder
	log      *logrus.Logger
}

// NewSearchService creates a SearchService.
func NewSearchService(store SearchStore, embedder Embedder, log *logrus.Logger) *SearchService {
	return &SearchService{store: store, embedder: embedder, log: log}
}

// FullTextSearch performs a full-text search (pass-through).
func (s *SearchService) FullTextSearch(
	ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int,
) ([]models.Node, error) {
	return s.store.FullTextSearch(ctx, tenantID, query, typeFilter, minSalience, limit)
}

// SemanticSearch generates an embedding from the query, then searches by vector similarity.
func (s *SearchService) SemanticSearch(
	ctx context.Context, tenantID, query string, limit int,
) ([]models.ScoredNode, error) {
	embedding, err := s.embedder.Generate(ctx, query)
	if err != nil {
		return nil, err
	}

	return s.store.SemanticSearch(ctx, tenantID, embedding, limit)
}

// HybridSearch generates an embedding from the query, then performs combined search.
// Returns the embedding error separately so the handler can decide on fallback.
func (s *SearchService) HybridSearch(
	ctx context.Context, tenantID, query string, limit int,
) ([]models.Node, error) {
	embedding, err := s.embedder.Generate(ctx, query)
	if err != nil {
		return nil, err
	}

	return s.store.HybridSearch(ctx, tenantID, query, embedding, limit)
}
