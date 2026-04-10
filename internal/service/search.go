package service

import (
	"context"
	"fmt"

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
	graph    GraphLookupStore
	embedder Embedder
	log      *logrus.Logger
}

// NewSearchService creates a SearchService.
func NewSearchService(store SearchStore, embedder Embedder, log *logrus.Logger) *SearchService {
	return &SearchService{store: store, embedder: embedder, log: log}
}

// WithGraphLookup enables graph-neighborhood expansion for retrieval.
func (s *SearchService) WithGraphLookup(graph GraphLookupStore) *SearchService {
	s.graph = graph
	return s
}

// FullTextSearch performs a full-text search (pass-through).
func (s *SearchService) FullTextSearch(
	ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int,
) ([]models.Node, error) {
	intent := DetectSearchIntent(query)
	adjustedMinSalience := minSalience
	if adjustedMinSalience <= 0 {
		switch intent {
		case SearchIntentProcedural:
			adjustedMinSalience = 1.0
		case SearchIntentTemporal:
			adjustedMinSalience = 0.5
		}
	}
	results, err := s.firstFullTextMatch(ctx, tenantID, BuildSearchQueryVariants(query), typeFilter, adjustedMinSalience, limit)
	if err != nil {
		return nil, err
	}
	results = mergeExpandedNodes(results, s.rescueByLabel(ctx, tenantID, query), limit)
	return mergeExpandedNodes(results, s.expandFromGraph(ctx, tenantID, results, limit), limit), nil
}

// SemanticSearch generates an embedding from the query, then searches by vector similarity.
func (s *SearchService) SemanticSearch(
	ctx context.Context, tenantID, query string, limit int,
) ([]models.ScoredNode, error) {
	variants := BuildSearchQueryVariants(query)
	if len(variants) == 0 {
		variants = []string{query}
	}

	embedding, err := s.embedder.Generate(ctx, variants[0])
	if err != nil {
		return nil, err
	}

	return s.store.SemanticSearch(ctx, tenantID, embedding, limit)
}

func (s *SearchService) firstFullTextMatch(
	ctx context.Context,
	tenantID string,
	queries []string,
	typeFilter string,
	minSalience float64,
	limit int,
) ([]models.Node, error) {
	var firstErr error
	for _, q := range queries {
		results, err := s.store.FullTextSearch(ctx, tenantID, q, typeFilter, minSalience, limit)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return []models.Node{}, nil
}

// HybridSearch generates an embedding from the query, then performs combined search.
// Returns the embedding error separately so the handler can decide on fallback.
func (s *SearchService) HybridSearch(
	ctx context.Context, tenantID, query string, limit int,
) ([]models.Node, error) {
	variants := BuildSearchQueryVariants(query)
	if len(variants) == 0 {
		variants = []string{query}
	}

	embedding, err := s.embedder.Generate(ctx, variants[0])
	if err != nil {
		return nil, err
	}

	searchLimit := limit
	if shouldPrototypeRerank(ctx, limit) {
		searchLimit = rerankCandidateLimit(limit)
	}

	var firstErr error
	for _, variant := range variants {
		results, searchErr := s.store.HybridSearch(ctx, tenantID, variant, embedding, searchLimit)
		if searchErr != nil {
			if firstErr == nil {
				firstErr = searchErr
			}
			continue
		}
		if len(results) > 0 {
			if shouldPrototypeRerank(ctx, limit) {
				results = prototypeRerankNodesWithProfile(query, results, limit, InternalRerankProfile(ctx))
			}
			results = mergeExpandedNodes(results, s.rescueByLabel(ctx, tenantID, query), limit)
			return mergeExpandedNodes(results, s.expandFromGraph(ctx, tenantID, results, limit), limit), nil
		}
	}
	rescued := s.rescueByLabel(ctx, tenantID, query)
	if len(rescued) > 0 {
		return mergeExpandedNodes(rescued, s.expandFromGraph(ctx, tenantID, rescued, limit), limit), nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("hybrid search returned no results")
}
