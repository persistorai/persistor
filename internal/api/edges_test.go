package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/api"
	"github.com/persistorai/persistor/internal/models"
)

func TestEdgeCreate_Valid(t *testing.T) {
	t.Parallel()

	repo := &mockEdgeRepo{
		createFn: func(_ context.Context, _ string, req models.CreateEdgeRequest) (*models.Edge, error) {
			return &models.Edge{
				Source:    req.Source,
				Target:    req.Target,
				Relation:  req.Relation,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	r := newTestRouter()
	h := api.NewEdgeHandler(repo, testLogger())
	r.POST("/edges", h.Create)

	w := doRequest(r, http.MethodPost, "/edges", `{"source":"a","target":"b","relation":"knows"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var edge models.Edge
	if err := json.Unmarshal(w.Body.Bytes(), &edge); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if edge.Source != "a" || edge.Target != "b" {
		t.Errorf("unexpected edge: %+v", edge)
	}
}

func TestEdgeCreate_MissingSource(t *testing.T) {
	t.Parallel()

	r := newTestRouter()
	h := api.NewEdgeHandler(&mockEdgeRepo{}, testLogger())
	r.POST("/edges", h.Create)

	w := doRequest(r, http.MethodPost, "/edges", `{"target":"b","relation":"knows"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEdgeDelete_OK(t *testing.T) {
	t.Parallel()

	repo := &mockEdgeRepo{
		deleteFn: func(_ context.Context, _, _, _, _ string) error {
			return nil
		},
	}

	r := newTestRouter()
	h := api.NewEdgeHandler(repo, testLogger())
	r.DELETE("/edges/:source/:target/:relation", h.Delete)

	w := doRequest(r, http.MethodDelete, "/edges/a/b/knows", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", body["deleted"])
	}
}
