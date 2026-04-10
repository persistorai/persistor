package api_test

import (
	"context"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

// mockNodeRepo implements api.NodeService for testing.
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

func (m *mockNodeRepo) GetNodeByLabel(_ context.Context, _, _ string) (*models.Node, error) {
	return nil, nil
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

func (m *mockNodeRepo) PatchNodeProperties(_ context.Context, _, _ string, _ models.PatchPropertiesRequest) (*models.Node, error) {
	return nil, nil
}

func (m *mockNodeRepo) MigrateNode(_ context.Context, _, _ string, _ models.MigrateNodeRequest) (*models.MigrateNodeResult, error) {
	return nil, nil
}

// mockEdgeRepo implements api.EdgeService for testing.
type mockEdgeRepo struct {
	listFn   func(ctx context.Context, tenantID, source, target, relation string, limit, offset int, activeOn *time.Time, current *bool) ([]models.Edge, bool, error)
	createFn func(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	updateFn func(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	deleteFn func(ctx context.Context, tenantID, source, target, relation string) error
}

func (m *mockEdgeRepo) ListEdges(ctx context.Context, tenantID, source, target, relation string, limit, offset int, activeOn *time.Time, current *bool) ([]models.Edge, bool, error) {
	return m.listFn(ctx, tenantID, source, target, relation, limit, offset, activeOn, current)
}

func (m *mockEdgeRepo) CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error) { //nolint:gocritic // hugeParam: matches domain.EdgeService interface signature
	return m.createFn(ctx, tenantID, req)
}

func (m *mockEdgeRepo) UpdateEdge(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error) {
	return m.updateFn(ctx, tenantID, source, target, relation, req)
}

func (m *mockEdgeRepo) PatchEdgeProperties(_ context.Context, _, _, _, _ string, _ models.PatchPropertiesRequest) (*models.Edge, error) {
	return nil, nil
}

func (m *mockEdgeRepo) DeleteEdge(ctx context.Context, tenantID, source, target, relation string) error {
	return m.deleteFn(ctx, tenantID, source, target, relation)
}

// mockSearchRepo implements api.SearchService for testing.
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

type mockAdminRepo struct {
	recordFeedbackFn func(ctx context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error)
	summaryFn        func(ctx context.Context, tenantID string, opts models.RetrievalFeedbackListOpts) (*models.RetrievalFeedbackSummary, error)
}

func (m *mockAdminRepo) ListNodesWithoutEmbeddings(_ context.Context, _ string, _ int) ([]models.NodeSummary, error) {
	return nil, nil
}

func (m *mockAdminRepo) ReprocessNodes(_ context.Context, _ string, _ models.ReprocessNodesRequest) (*models.ReprocessNodesResult, error) {
	return nil, nil
}

func (m *mockAdminRepo) RunMaintenance(_ context.Context, _ string, _ models.MaintenanceRunRequest) (*models.MaintenanceRunResult, error) {
	return nil, nil
}

func (m *mockAdminRepo) ListMergeSuggestions(_ context.Context, _ string, _ models.MergeSuggestionListOpts) ([]models.MergeSuggestion, error) {
	return nil, nil
}

func (m *mockAdminRepo) RecordRetrievalFeedback(ctx context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
	return m.recordFeedbackFn(ctx, tenantID, req)
}

func (m *mockAdminRepo) GetRetrievalFeedbackSummary(ctx context.Context, tenantID string, opts models.RetrievalFeedbackListOpts) (*models.RetrievalFeedbackSummary, error) {
	return m.summaryFn(ctx, tenantID, opts)
}
