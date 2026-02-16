package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/persistorai/persistor/internal/api"
	"github.com/persistorai/persistor/internal/models"
)

func TestFullTextSearch_OK(t *testing.T) {
	t.Parallel()

	repo := &mockSearchRepo{
		fullTextFn: func(_ context.Context, _, query, _ string, _ float64, _ int) ([]models.Node, error) {
			return []models.Node{{ID: "n1", Type: "person", Label: query}}, nil
		},
	}

	r := newTestRouter()
	h := api.NewSearchHandler(repo, testLogger())
	r.GET("/search", h.FullText)

	w := doRequest(r, http.MethodGet, "/search?q=test", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	nodes, ok := body["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Errorf("expected 1 result, got %v", body["nodes"])
	}
}

func TestFullTextSearch_MissingQ(t *testing.T) {
	t.Parallel()

	r := newTestRouter()
	h := api.NewSearchHandler(&mockSearchRepo{}, testLogger())
	r.GET("/search", h.FullText)

	w := doRequest(r, http.MethodGet, "/search", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSemanticSearch_OK(t *testing.T) {
	t.Parallel()

	repo := &mockSearchRepo{
		semanticFn: func(_ context.Context, _, _ string, _ int) ([]models.ScoredNode, error) {
			return []models.ScoredNode{
				{Node: models.Node{ID: "n1", Type: "concept", Label: "test"}, Score: 0.95},
			}, nil
		},
	}

	r := newTestRouter()
	h := api.NewSearchHandler(repo, testLogger())
	r.GET("/search/semantic", h.Semantic)

	w := doRequest(r, http.MethodGet, "/search/semantic?q=test", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHybridSearch_OK(t *testing.T) {
	t.Parallel()

	repo := &mockSearchRepo{
		hybridFn: func(_ context.Context, _, _ string, _ int) ([]models.Node, error) {
			return []models.Node{{ID: "n1", Type: "concept", Label: "test"}}, nil
		},
	}

	r := newTestRouter()
	h := api.NewSearchHandler(repo, testLogger())
	r.GET("/search/hybrid", h.Hybrid)

	w := doRequest(r, http.MethodGet, "/search/hybrid?q=test", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
