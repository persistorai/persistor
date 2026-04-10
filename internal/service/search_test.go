package service

import (
	"context"
	"errors"
	"testing"

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
