package service

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestCandidateLabelsFromQuery(t *testing.T) {
	t.Parallel()

	labels := candidateLabelsFromQuery("What is Big Jerry connected to?")
	if len(labels) == 0 {
		t.Fatal("expected candidate labels")
	}
	if !containsString(labels, "Big Jerry") {
		t.Fatalf("expected Big Jerry candidate, got %v", labels)
	}
}

func TestSearchService_RescueByLabel(t *testing.T) {
	t.Parallel()

	store := &mockSearchStore{
		fullTextSearch: func(_ context.Context, _, _, _ string, _ float64, _ int) ([]models.Node, error) {
			return []models.Node{}, nil
		},
		hybridSearch: func(_ context.Context, _, _ string, _ []float32, _ int) ([]models.Node, error) {
			return []models.Node{}, nil
		},
		semanticSearch: func(_ context.Context, _ string, _ []float32, _ int) ([]models.ScoredNode, error) {
			return []models.ScoredNode{}, nil
		},
		getNodeByLabel: func(_ context.Context, _, label string) (*models.Node, error) {
			if label == "Persistor" {
				return &models.Node{ID: "persistor", Label: "Persistor", Type: "project", Salience: 50}, nil
			}
			return nil, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewSearchService(store, &mockEmbedder{generate: func(_ context.Context, _ string) ([]float32, error) {
		return []float32{0.1}, nil
	}}, log)

	results, err := svc.HybridSearch(context.Background(), "t1", "What is Persistor?", 5)
	if err == nil {
		if len(results) == 0 || results[0].Label != "Persistor" {
			t.Fatalf("expected label rescue result, got %v", results)
		}
		return
	}
	t.Fatalf("unexpected error: %v", err)
}
