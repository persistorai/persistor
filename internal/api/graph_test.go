package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/persistorai/persistor/internal/api"
	"github.com/persistorai/persistor/internal/models"
)

type mockGraphRepo struct {
	neighborsFn    func(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	traverseFn     func(ctx context.Context, tenantID, nodeID string, maxHops int) (*models.TraverseResult, error)
	graphContextFn func(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error)
	shortestPathFn func(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error)
}

func (m *mockGraphRepo) Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error) {
	return m.neighborsFn(ctx, tenantID, nodeID, limit)
}

func (m *mockGraphRepo) Traverse(ctx context.Context, tenantID, nodeID string, maxHops int) (*models.TraverseResult, error) {
	return m.traverseFn(ctx, tenantID, nodeID, maxHops)
}

func (m *mockGraphRepo) GraphContext(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error) {
	return m.graphContextFn(ctx, tenantID, nodeID)
}

func (m *mockGraphRepo) ShortestPath(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error) {
	return m.shortestPathFn(ctx, tenantID, fromID, toID)
}

func TestGraphPathMissingNodeReturns404(t *testing.T) {
	r := newTestRouter()
	h := api.NewGraphHandler(&mockGraphRepo{
		neighborsFn:    func(context.Context, string, string, int) (*models.NeighborResult, error) { return nil, nil },
		traverseFn:     func(context.Context, string, string, int) (*models.TraverseResult, error) { return nil, nil },
		graphContextFn: func(context.Context, string, string) (*models.ContextResult, error) { return nil, nil },
		shortestPathFn: func(context.Context, string, string, string) ([]models.Node, error) {
			return nil, models.ErrNodeNotFound
		},
	}, testLogger())
	r.GET("/graph/path/:from/:to", h.Path)

	w := doRequest(r, http.MethodGet, "/graph/path/a/b", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
