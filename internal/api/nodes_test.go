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

func TestNodeCreate_Valid(t *testing.T) {
	t.Parallel()

	repo := &mockNodeRepo{
		createFn: func(_ context.Context, _ string, req models.CreateNodeRequest) (*models.Node, error) {
			return &models.Node{
				ID:        req.ID,
				Type:      req.Type,
				Label:     req.Label,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	r := newTestRouter()
	h := api.NewNodeHandler(repo, testLogger())
	r.POST("/nodes", h.Create)

	w := doRequest(r, http.MethodPost, "/nodes", `{"id":"n1","type":"person","label":"Alice"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var node models.Node
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if node.ID != "n1" {
		t.Errorf("expected id 'n1', got %q", node.ID)
	}
}

func TestNodeCreate_MissingType(t *testing.T) {
	t.Parallel()

	r := newTestRouter()
	h := api.NewNodeHandler(&mockNodeRepo{}, testLogger())
	r.POST("/nodes", h.Create)

	w := doRequest(r, http.MethodPost, "/nodes", `{"id":"n1","label":"Alice"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNodeGet_Found(t *testing.T) {
	t.Parallel()

	repo := &mockNodeRepo{
		getFn: func(_ context.Context, _ string, nodeID string) (*models.Node, error) {
			return &models.Node{ID: nodeID, Type: "person", Label: "Alice"}, nil
		},
	}

	r := newTestRouter()
	h := api.NewNodeHandler(repo, testLogger())
	r.GET("/nodes/:id", h.Get)

	w := doRequest(r, http.MethodGet, "/nodes/n1", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var node models.Node
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if node.ID != "n1" {
		t.Errorf("expected id 'n1', got %q", node.ID)
	}
}

func TestNodeGet_NotFound(t *testing.T) {
	t.Parallel()

	repo := &mockNodeRepo{
		getFn: func(_ context.Context, _, _ string) (*models.Node, error) {
			return nil, models.ErrNodeNotFound
		},
	}

	r := newTestRouter()
	h := api.NewNodeHandler(repo, testLogger())
	r.GET("/nodes/:id", h.Get)

	w := doRequest(r, http.MethodGet, "/nodes/missing", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNodeUpdate_OK(t *testing.T) {
	t.Parallel()

	repo := &mockNodeRepo{
		updateFn: func(_ context.Context, _, nodeID string, _ models.UpdateNodeRequest) (*models.Node, error) {
			return &models.Node{ID: nodeID, Type: "person", Label: "Updated"}, nil
		},
	}

	r := newTestRouter()
	h := api.NewNodeHandler(repo, testLogger())
	r.PUT("/nodes/:id", h.Update)

	w := doRequest(r, http.MethodPut, "/nodes/n1", `{"label":"Updated"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNodeDelete_OK(t *testing.T) {
	t.Parallel()

	repo := &mockNodeRepo{
		deleteFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	r := newTestRouter()
	h := api.NewNodeHandler(repo, testLogger())
	r.DELETE("/nodes/:id", h.Delete)

	w := doRequest(r, http.MethodDelete, "/nodes/n1", "")

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
