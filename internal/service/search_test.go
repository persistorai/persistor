package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestSearchService_FullTextSearch(t *testing.T) {
	queries := make([]string, 0, 4)
	store := &mockSearchStore{
		fullTextSearch: func(_ context.Context, _, query, _ string, _ float64, _ int) ([]models.Node, error) {
			queries = append(queries, query)
			if query == "big jerry" {
				return []models.Node{{ID: "n1", Label: "Big Jerry", Salience: 10}}, nil
			}
			return []models.Node{}, nil
		},
	}
	graph := &mockGraphLookupStore{
		neighbors: func(_ context.Context, _, nodeID string, _ int) (*models.NeighborResult, error) {
			if nodeID != "n1" {
				return &models.NeighborResult{}, nil
			}
			return &models.NeighborResult{Nodes: []models.Node{{ID: "n2", Label: "Oklahoma", Salience: 20}}}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, nil, log).WithGraphLookup(graph)

	nodes, err := svc.FullTextSearch(context.Background(), "t1", "Who is Big Jerry?", "", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n2" {
		t.Errorf("unexpected results: %v", nodes)
	}
	if len(store.calls) < 2 || store.calls[0] != "FullTextSearch" || store.calls[1] != "FullTextSearch" {
		t.Errorf("expected repeated FullTextSearch calls first, got %v", store.calls)
	}
	if len(queries) < 2 || queries[1] != "big jerry" {
		t.Fatalf("expected fallback variant query, got %v", queries)
	}
}

func TestSearchService_SemanticSearch(t *testing.T) {
	tests := []struct {
		name     string
		embedErr error
		storeErr error
		wantErr  bool
	}{
		{name: "success"},
		{name: "embed error", embedErr: errors.New("ollama down"), wantErr: true},
		{name: "store error", storeErr: errors.New("db error"), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			embedder := &mockEmbedder{
				generate: func(_ context.Context, _ string) ([]float32, error) {
					if tc.embedErr != nil {
						return nil, tc.embedErr
					}
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}
			store := &mockSearchStore{
				semanticSearch: func(_ context.Context, _ string, _ []float32, _ int) ([]models.ScoredNode, error) {
					if tc.storeErr != nil {
						return nil, tc.storeErr
					}
					return []models.ScoredNode{{Node: models.Node{ID: "n1"}, Score: 0.95}}, nil
				},
			}
			log := logrus.New()
			log.SetLevel(logrus.ErrorLevel)
			svc := NewSearchService(store, embedder, log)

			results, err := svc.SemanticSearch(context.Background(), "t1", "test query", 10)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != 1 || results[0].Score != 0.95 {
				t.Errorf("unexpected results: %v", results)
			}
		})
	}
}

func TestSearchService_HybridSearch(t *testing.T) {
	tests := []struct {
		name     string
		embedErr error
		wantErr  bool
	}{
		{name: "success"},
		{name: "embed error", embedErr: errors.New("fail"), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			embedder := &mockEmbedder{
				generate: func(_ context.Context, _ string) ([]float32, error) {
					if tc.embedErr != nil {
						return nil, tc.embedErr
					}
					return []float32{0.1, 0.2}, nil
				},
			}
			queries := make([]string, 0, 4)
			store := &mockSearchStore{
				hybridSearch: func(_ context.Context, _, query string, _ []float32, _ int) ([]models.Node, error) {
					queries = append(queries, query)
					if tc.wantErr {
						return nil, nil
					}
					if query == "big jerry" {
						return []models.Node{{ID: "n1"}, {ID: "n2"}}, nil
					}
					return []models.Node{}, nil
				},
			}
			graph := &mockGraphLookupStore{
				neighbors: func(_ context.Context, _, nodeID string, _ int) (*models.NeighborResult, error) {
					if nodeID != "n1" {
						return &models.NeighborResult{}, nil
					}
					return &models.NeighborResult{Nodes: []models.Node{{ID: "n3", Label: "Ridge Line 2", Salience: 30}}}, nil
				},
			}
			log := logrus.New()
			log.SetLevel(logrus.ErrorLevel)
			svc := NewSearchService(store, embedder, log).WithGraphLookup(graph)

			nodes, err := svc.HybridSearch(context.Background(), "t1", "Who is Big Jerry?", 10)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(nodes) != 3 {
				t.Errorf("got %d nodes, want 3", len(nodes))
			}
			if nodes[2].ID != "n3" {
				t.Fatalf("expected graph-expanded node n3, got %v", nodes)
			}
			if len(queries) < 2 || queries[1] != "big jerry" {
				t.Fatalf("expected hybrid fallback variant, got %v", queries)
			}
		})
	}
}

func TestSearchService_HybridSearch_PrototypeReranksCandidates(t *testing.T) {
	now := time.Now()
	embedder := &mockEmbedder{
		generate: func(_ context.Context, _ string) ([]float32, error) {
			return []float32{0.1, 0.2}, nil
		},
	}

	var receivedLimit int
	store := &mockSearchStore{
		hybridSearch: func(_ context.Context, _, _ string, _ []float32, limit int) ([]models.Node, error) {
			receivedLimit = limit
			return []models.Node{
				{ID: "n1", Label: "Deployment log", Type: "note", Properties: map[string]any{"summary": "Unrelated maintenance"}, Salience: 95, UpdatedAt: now.Add(-time.Hour)},
				{ID: "n2", Label: "Persistor deployment fix", Type: "incident", Properties: map[string]any{"summary": "Resolved Persistor deploy issue in production"}, Salience: 20, UpdatedAt: now},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, embedder, log)

	ctx := WithInternalRerankMode(context.Background(), "prototype")
	nodes, err := svc.HybridSearch(ctx, "t1", "Persistor deploy fix", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 3 {
		t.Fatalf("expected rerank candidate overfetch limit 3, got %d", receivedLimit)
	}
	if len(nodes) != 1 || nodes[0].ID != "n2" {
		t.Fatalf("expected reranked top result n2, got %#v", nodes)
	}
}

func TestSearchService_FullTextSearch_BeliefAwareShaping(t *testing.T) {
	store := &mockSearchStore{
		fullTextSearch: func(_ context.Context, _, query, _ string, _ float64, _ int) ([]models.Node, error) {
			if query != "alice" {
				return []models.Node{}, nil
			}
			return []models.Node{
				{
					ID:       "n1",
					Label:    "Alice profile draft",
					Salience: 90,
					Properties: map[string]any{
						"city":                     "Chicago",
						models.FactBeliefsProperty: map[string]any{"city": map[string]any{"preferred_value": "Chicago", "preferred_confidence": 0.62, "evidence_count": 2, "status": models.FactBeliefStatusContested, "claims": []map[string]any{{"value": "Chicago", "preferred": true, "last_observed_at": "2026-03-01T12:00:00Z"}, {"value": "Austin", "last_observed_at": "2026-03-05T12:00:00Z"}}}},
					},
				},
				{
					ID:       "n2",
					Label:    "Alice",
					Salience: 20,
					Properties: map[string]any{
						"city":                     "Chicago",
						models.FactBeliefsProperty: map[string]any{"city": map[string]any{"preferred_value": "Chicago", "preferred_confidence": 0.96, "evidence_count": 4, "status": models.FactBeliefStatusSupported, "claims": []map[string]any{{"value": "Chicago", "preferred": true, "last_observed_at": "2026-04-10T12:00:00Z"}}}},
					},
				},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, nil, log)

	nodes, err := svc.FullTextSearch(context.Background(), "t1", "Alice", "", 0, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n2" {
		t.Fatalf("expected supported belief node n2 first, got %#v", nodes)
	}
}

func TestSearchService_HybridSearch_BeliefAwareShaping(t *testing.T) {
	embedder := &mockEmbedder{generate: func(_ context.Context, _ string) ([]float32, error) { return []float32{0.1, 0.2}, nil }}
	store := &mockSearchStore{
		hybridSearch: func(_ context.Context, _, query string, _ []float32, _ int) ([]models.Node, error) {
			if query != "release plan" {
				return []models.Node{}, nil
			}
			return []models.Node{
				{
					ID:       "n1",
					Label:    "Release plan archive",
					Salience: 80,
					Properties: map[string]any{
						models.FactBeliefsProperty: map[string]any{"status": map[string]any{"preferred_value": "complete", "preferred_confidence": 0.4, "evidence_count": 1, "status": models.FactBeliefStatusSuperseded, "claims": []map[string]any{{"value": "complete", "preferred": true, "last_observed_at": "2025-01-01T12:00:00Z"}}}},
					},
				},
				{
					ID:       "n2",
					Label:    "Release plan",
					Salience: 30,
					Properties: map[string]any{
						"status":                   "active",
						models.FactBeliefsProperty: map[string]any{"status": map[string]any{"preferred_value": "active", "preferred_confidence": 0.9, "evidence_count": 3, "status": models.FactBeliefStatusSupported, "claims": []map[string]any{{"value": "active", "preferred": true, "last_observed_at": "2026-04-10T12:00:00Z"}}}},
					},
				},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, embedder, log)

	nodes, err := svc.HybridSearch(context.Background(), "t1", "release plan", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n2" {
		t.Fatalf("expected supported current node n2 first, got %#v", nodes)
	}
}

func TestSearchService_FullTextSearch_TemporalShapingPrefersRecent(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	store := &mockSearchStore{
		fullTextSearch: func(_ context.Context, _, query, _ string, _ float64, _ int) ([]models.Node, error) {
			if query != "current deployment status" {
				return []models.Node{}, nil
			}
			return []models.Node{
				{ID: "n1", Label: "Deployment status archive", Salience: 90, UpdatedAt: now.Add(-30 * 24 * time.Hour)},
				{ID: "n2", Label: "Deployment status current", Salience: 20, UpdatedAt: now.Add(-2 * time.Hour)},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, nil, log)

	nodes, err := svc.FullTextSearch(context.Background(), "t1", "current deployment status", "", 0, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n2" {
		t.Fatalf("expected recent node n2 first, got %#v", nodes)
	}
}

func TestSearchService_HybridSearch_TemporalShapingPrefersHistoricalYearMatch(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	embedder := &mockEmbedder{generate: func(_ context.Context, _ string) ([]float32, error) { return []float32{0.1, 0.2}, nil }}
	store := &mockSearchStore{
		hybridSearch: func(_ context.Context, _, query string, _ []float32, _ int) ([]models.Node, error) {
			if query != "history of platform migration 2024" {
				return []models.Node{}, nil
			}
			return []models.Node{
				{ID: "n1", Label: "Platform migration recap", Salience: 80, UpdatedAt: now, Properties: map[string]any{"summary": "Recap published in 2026"}},
				{ID: "n2", Label: "Platform migration", Salience: 25, UpdatedAt: now.Add(-365 * 24 * time.Hour), Properties: map[string]any{"happened_on": "2024-06-10", "summary": "Initial migration completed in 2024"}},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, embedder, log)

	nodes, err := svc.HybridSearch(context.Background(), "t1", "history of platform migration 2024", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n2" {
		t.Fatalf("expected historical year-matched node n2 first, got %#v", nodes)
	}
}

func TestSearchService_HybridSearch_PrototypeReranksWithProfile(t *testing.T) {
	now := time.Now()
	embedder := &mockEmbedder{generate: func(_ context.Context, _ string) ([]float32, error) { return []float32{0.1, 0.2}, nil }}
	store := &mockSearchStore{
		hybridSearch: func(_ context.Context, _, _ string, _ []float32, _ int) ([]models.Node, error) {
			return []models.Node{
				{ID: "n1", Label: "Persistor deploy", Type: "note", Properties: map[string]any{"summary": "Operational notes"}, Salience: 140, UpdatedAt: now.Add(-time.Hour)},
				{ID: "n2", Label: "Incident notes", Type: "incident", Properties: map[string]any{"summary": "Persistor deploy fix remediation"}, Salience: 20, UserBoosted: true, UpdatedAt: now.Add(-2 * time.Hour)},
			}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, embedder, log)

	baselineCtx := WithInternalRerankMode(context.Background(), "prototype")
	baseline, err := svc.HybridSearch(baselineCtx, "t1", "Persistor deploy fix remediation", 1)
	if err != nil {
		t.Fatalf("unexpected baseline error: %v", err)
	}
	if len(baseline) != 1 || baseline[0].ID != "n1" {
		t.Fatalf("expected default profile to keep n1 first, got %#v", baseline)
	}

	profileCtx := WithInternalRerankProfile(baselineCtx, "term_focus")
	weighted, err := svc.HybridSearch(profileCtx, "t1", "Persistor deploy fix remediation", 1)
	if err != nil {
		t.Fatalf("unexpected weighted error: %v", err)
	}
	if len(weighted) != 1 || weighted[0].ID != "n2" {
		t.Fatalf("expected term_focus profile to promote n2, got %#v", weighted)
	}
}
