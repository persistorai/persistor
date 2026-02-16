package service

import (
	"context"
	"sync"

	"github.com/persistorai/persistor/internal/models"
)

// mockNodeStore records calls and returns configured responses.
type mockNodeStore struct {
	mu    sync.Mutex
	calls []string

	listNodes  func(ctx context.Context, tenantID, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error)
	getNode    func(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	createNode func(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error)
	updateNode func(ctx context.Context, tenantID, nodeID string, req models.UpdateNodeRequest) (*models.Node, error)
	deleteNode func(ctx context.Context, tenantID, nodeID string) error
}

func (m *mockNodeStore) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

func (m *mockNodeStore) ListNodes(ctx context.Context, tenantID, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error) {
	m.record("ListNodes")
	return m.listNodes(ctx, tenantID, typeFilter, minSalience, limit, offset)
}

func (m *mockNodeStore) GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	m.record("GetNode")
	return m.getNode(ctx, tenantID, nodeID)
}

func (m *mockNodeStore) CreateNode(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error) {
	m.record("CreateNode")
	return m.createNode(ctx, tenantID, req)
}

func (m *mockNodeStore) UpdateNode(ctx context.Context, tenantID, nodeID string, req models.UpdateNodeRequest) (*models.Node, error) {
	m.record("UpdateNode")
	return m.updateNode(ctx, tenantID, nodeID, req)
}

func (m *mockNodeStore) DeleteNode(ctx context.Context, tenantID, nodeID string) error {
	m.record("DeleteNode")
	return m.deleteNode(ctx, tenantID, nodeID)
}

// mockEdgeStore records calls and returns configured responses.
type mockEdgeStore struct {
	mu    sync.Mutex
	calls []string

	listEdges  func(ctx context.Context, tenantID, source, target, relation string, limit, offset int) ([]models.Edge, bool, error)
	createEdge func(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	updateEdge func(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	deleteEdge func(ctx context.Context, tenantID, source, target, relation string) error
}

func (m *mockEdgeStore) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

func (m *mockEdgeStore) ListEdges(ctx context.Context, tenantID, source, target, relation string, limit, offset int) ([]models.Edge, bool, error) {
	m.record("ListEdges")
	return m.listEdges(ctx, tenantID, source, target, relation, limit, offset)
}

func (m *mockEdgeStore) CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error) {
	m.record("CreateEdge")
	return m.createEdge(ctx, tenantID, req)
}

func (m *mockEdgeStore) UpdateEdge(ctx context.Context, tenantID, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error) {
	m.record("UpdateEdge")
	return m.updateEdge(ctx, tenantID, source, target, relation, req)
}

func (m *mockEdgeStore) DeleteEdge(ctx context.Context, tenantID, source, target, relation string) error {
	m.record("DeleteEdge")
	return m.deleteEdge(ctx, tenantID, source, target, relation)
}

// mockSearchStore records calls and returns configured responses.
type mockSearchStore struct {
	mu    sync.Mutex
	calls []string

	fullTextSearch func(ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	semanticSearch func(ctx context.Context, tenantID string, embedding []float32, limit int) ([]models.ScoredNode, error)
	hybridSearch   func(ctx context.Context, tenantID, query string, embedding []float32, limit int) ([]models.Node, error)
}

func (m *mockSearchStore) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

func (m *mockSearchStore) FullTextSearch(ctx context.Context, tenantID, query, typeFilter string, minSalience float64, limit int) ([]models.Node, error) {
	m.record("FullTextSearch")
	return m.fullTextSearch(ctx, tenantID, query, typeFilter, minSalience, limit)
}

func (m *mockSearchStore) SemanticSearch(ctx context.Context, tenantID string, embedding []float32, limit int) ([]models.ScoredNode, error) {
	m.record("SemanticSearch")
	return m.semanticSearch(ctx, tenantID, embedding, limit)
}

func (m *mockSearchStore) HybridSearch(ctx context.Context, tenantID, query string, embedding []float32, limit int) ([]models.Node, error) {
	m.record("HybridSearch")
	return m.hybridSearch(ctx, tenantID, query, embedding, limit)
}

// mockEmbedder returns configured embeddings.
type mockEmbedder struct {
	generate func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockEmbedder) Generate(ctx context.Context, text string) ([]float32, error) {
	return m.generate(ctx, text)
}

// mockAuditor records audit calls.
type mockAuditor struct {
	mu    sync.Mutex
	calls []AuditJob

	err error
}

func (m *mockAuditor) RecordAudit(ctx context.Context, tenantID, action, entityType, entityID, actor string, detail map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, AuditJob{
		TenantID:   tenantID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Actor:      actor,
		Detail:     detail,
	})
	return m.err
}

func (m *mockAuditor) getCalls() []AuditJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]AuditJob, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// mockEmbedEnqueuer records enqueue calls.
type mockEmbedEnqueuer struct {
	mu   sync.Mutex
	jobs []EmbedJob
}

func (m *mockEmbedEnqueuer) Enqueue(job EmbedJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = append(m.jobs, job)
}
