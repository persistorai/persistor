package service

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestSearchService_FullTextSearch(t *testing.T) {
	store := &mockSearchStore{
		fullTextSearch: func(_ context.Context, _, _, _ string, _ float64, _ int) ([]models.Node, error) {
			return []models.Node{{ID: "n1", Label: "Match"}}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, nil, log)

	nodes, err := svc.FullTextSearch(context.Background(), "t1", "match", "", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "n1" {
		t.Errorf("unexpected results: %v", nodes)
	}
	if len(store.calls) != 1 || store.calls[0] != "FullTextSearch" {
		t.Errorf("expected FullTextSearch call, got %v", store.calls)
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
			store := &mockSearchStore{
				hybridSearch: func(_ context.Context, _, _ string, _ []float32, _ int) ([]models.Node, error) {
					return []models.Node{{ID: "n1"}, {ID: "n2"}}, nil
				},
			}
			log := logrus.New()
			log.SetLevel(logrus.ErrorLevel)
			svc := NewSearchService(store, embedder, log)

			nodes, err := svc.HybridSearch(context.Background(), "t1", "query", 10)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(nodes) != 2 {
				t.Errorf("got %d nodes, want 2", len(nodes))
			}
		})
	}
}
