package api_test

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// mockNodeRepo implements api.NodeRepository for testing.
type mockNodeRepo struct {
	listFn   func(ctx context.Context, tenantID, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error)
	getFn    func(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	createFn func(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error)
	updateFn func(ctx context.Context, tenantID, nodeID string, req models.UpdateNodeRequest) (*models.Node, error)
	deleteFn func(ctx context.Context, tenantID, nodeID string) error
}

func (m *mockNodeRepo) ListNodes(ctx context.Context, tenantID, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error) {
	return m.listFn(ctx, tenantID, typeFilter, minSalience, limit, offset)
}

func (m *mockNodeRepo) GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	return m.getFn(ctx, tenantID, nodeID)
}

func (m *mockNodeRepo) CreateNode(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error) {
	return m.createFn(ctx, tenantID, req)
}

func (m *mockNodeRepo) UpdateNode(ctx context.Context, tenantID, nodeID string, req models.UpdateNodeRequest) (*models.Node, error) {
	return m.updateFn(ctx, tenantID, nodeID, req)
}

func (m *mockNodeRepo) DeleteNode(ctx context.Context, tenantID, nodeID string) error {
	return m.deleteFn(ctx, tenantID, nodeID)
}

// mockEdgeRepo implements api.EdgeRepository for testing.
type mockEdgeRepo struct {
	listFn   func(ctx context.Context, tenantID, source, target, relation string, limit, offset int) ([]models.Edge, bool, error)
	createFn func(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	updateFn func(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	deleteFn func(ctx context.Context, tenantID, source, target, relation string) error
}

func (m *mockEdgeRepo) ListEdges(ctx context.Context, tenantID, source, target, relation string, limit, offset int) ([]models.Edge, bool, error) {
	return m.listFn(ctx, tenantID, source, target, relation, limit, offset)
}

func (m *mockEdgeRepo) CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error) {
	return m.createFn(ctx, tenantID, req)
}

func (m *mockEdgeRepo) UpdateEdge(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error) {
	return m.updateFn(ctx, tenantID, source, target, relation, req)
}

func (m *mockEdgeRepo) DeleteEdge(ctx context.Context, tenantID, source, target, relation string) error {
	return m.deleteFn(ctx, tenantID, source, target, relation)
}

// mockSearchRepo implements api.SearchRepository for testing.
type mockSearchRepo struct {
	fullTextFn func(ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	semanticFn func(ctx context.Context, tenantID, query string, limit int) ([]models.ScoredNode, error)
	hybridFn   func(ctx context.Context, tenantID, query string, limit int) ([]models.Node, error)
}

func (m *mockSearchRepo) FullTextSearch(ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int) ([]models.Node, error) {
	return m.fullTextFn(ctx, tenantID, query, typeFilter, minSalience, limit)
}

func (m *mockSearchRepo) SemanticSearch(ctx context.Context, tenantID, query string, limit int) ([]models.ScoredNode, error) {
	return m.semanticFn(ctx, tenantID, query, limit)
}

func (m *mockSearchRepo) HybridSearch(ctx context.Context, tenantID, query string, limit int) ([]models.Node, error) {
	return m.hybridFn(ctx, tenantID, query, limit)
}
