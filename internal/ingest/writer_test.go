package ingest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/persistorai/persistor/client"
	"github.com/persistorai/persistor/internal/ingest"
)

// mockGraphClient implements ingest.GraphClient for testing.
type mockGraphClient struct {
	labelNodes   map[string]*client.Node
	searchNodes  []client.Node
	createdNodes []client.CreateNodeRequest
	patchedProps map[string]map[string]any
	createdEdges []client.CreateEdgeRequest
	nodeIDSeq    int
	createErr    error
}

func newMockGraphClient() *mockGraphClient {
	return &mockGraphClient{
		labelNodes:   make(map[string]*client.Node),
		patchedProps: make(map[string]map[string]any),
	}
}

func (m *mockGraphClient) GetNodeByLabel(_ context.Context, label string) (*client.Node, error) {
	return m.labelNodes[label], nil
}

func (m *mockGraphClient) SearchNodes(_ context.Context, _ string, _ int) ([]client.Node, error) {
	return m.searchNodes, nil
}

func (m *mockGraphClient) CreateNode(_ context.Context, req *client.CreateNodeRequest) (*client.Node, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.nodeIDSeq++
	m.createdNodes = append(m.createdNodes, *req)
	return &client.Node{ID: fmt.Sprintf("node-%d", m.nodeIDSeq), Label: req.Label}, nil
}

func (m *mockGraphClient) UpdateNode(_ context.Context, id string, req *client.UpdateNodeRequest) (*client.Node, error) {
	return &client.Node{ID: id}, nil
}

func (m *mockGraphClient) PatchNodeProperties(_ context.Context, id string, props map[string]any) (*client.Node, error) {
	m.patchedProps[id] = props
	return &client.Node{ID: id}, nil
}

func (m *mockGraphClient) CreateEdge(_ context.Context, req *client.CreateEdgeRequest) (*client.Edge, error) {
	m.createdEdges = append(m.createdEdges, *req)
	return &client.Edge{Source: req.Source, Target: req.Target, Relation: req.Relation}, nil
}

func TestWriteEntities_CreatesNewNodes(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	entities := []ingest.ExtractedEntity{
		{Name: "Alice", Type: "person", Properties: map[string]any{"role": "dev"}},
		{Name: "Persistor", Type: "project", Properties: map[string]any{}},
	}

	report, nodeMap, err := w.WriteEntities(context.Background(), entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.CreatedNodes != 2 {
		t.Errorf("expected 2 created nodes, got %d", report.CreatedNodes)
	}

	if len(nodeMap) != 2 {
		t.Errorf("expected 2 entries in nodeMap, got %d", len(nodeMap))
	}

	if _, ok := nodeMap["alice"]; !ok {
		t.Error("expected 'alice' in nodeMap")
	}
}

func TestWriteEntities_UpdatesExistingNode(t *testing.T) {
	gc := newMockGraphClient()
	gc.labelNodes["Alice"] = &client.Node{ID: "existing-1", Label: "Alice", Properties: map[string]any{"role": "manager"}}
	w := ingest.NewWriter(gc, "test-source")

	entities := []ingest.ExtractedEntity{
		{Name: "Alice", Type: "person", Properties: map[string]any{"role": "dev"}},
	}

	report, nodeMap, err := w.WriteEntities(context.Background(), entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.UpdatedNodes != 1 {
		t.Errorf("expected 1 updated node, got %d", report.UpdatedNodes)
	}

	if nodeMap["alice"] != "existing-1" {
		t.Errorf("expected node ID 'existing-1', got %q", nodeMap["alice"])
	}

	patched, ok := gc.patchedProps["existing-1"]
	if !ok {
		t.Fatal("expected properties to be patched on existing-1")
	}

	if patched["role"] != "dev" {
		t.Errorf("expected role=dev, got %v", patched["role"])
	}
}

func TestWriteRelationships_CanonicalCreatesEdge(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{"alice": "id-1", "persistor": "id-2"}
	rels := []ingest.ExtractedRelationship{
		{Source: "Alice", Target: "Persistor", Relation: "works_on", Confidence: 0.9},
	}

	report, err := w.WriteRelationships(context.Background(), rels, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.CreatedEdges != 1 {
		t.Errorf("expected 1 created edge, got %d", report.CreatedEdges)
	}

	if len(gc.createdEdges) != 1 {
		t.Fatalf("expected 1 edge call, got %d", len(gc.createdEdges))
	}

	if gc.createdEdges[0].Relation != "works_on" {
		t.Errorf("expected relation 'works_on', got %q", gc.createdEdges[0].Relation)
	}
}

func TestWriteRelationships_UnknownRelationSkipped(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{"alice": "id-1", "bob": "id-2"}
	rels := []ingest.ExtractedRelationship{
		{Source: "Alice", Target: "Bob", Relation: "invented_by", Confidence: 0.8},
	}

	report, err := w.WriteRelationships(context.Background(), rels, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.CreatedEdges != 0 {
		t.Errorf("expected 0 created edges, got %d", report.CreatedEdges)
	}

	if len(report.UnknownRelations) != 1 {
		t.Errorf("expected 1 unknown relation, got %d", len(report.UnknownRelations))
	}
}

func TestWriteRelationships_OrphanSkipped(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{"alice": "id-1"}
	rels := []ingest.ExtractedRelationship{
		{Source: "Alice", Target: "Unknown", Relation: "works_on", Confidence: 0.9},
	}

	report, err := w.WriteRelationships(context.Background(), rels, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.SkippedEdges != 1 {
		t.Errorf("expected 1 skipped edge, got %d", report.SkippedEdges)
	}
}

func TestWriteFacts_PatchesProperty(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{"alice": "id-1"}
	facts := []ingest.ExtractedFact{
		{Subject: "Alice", Property: "age", Value: "30"},
	}

	err := w.WriteFacts(context.Background(), facts, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patched, ok := gc.patchedProps["id-1"]
	if !ok {
		t.Fatal("expected properties to be patched on id-1")
	}

	if patched["age"] != "30" {
		t.Errorf("expected age=30, got %v", patched["age"])
	}
}

func TestWriteFacts_UnknownSubjectSkipped(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{}
	facts := []ingest.ExtractedFact{
		{Subject: "Unknown", Property: "age", Value: "30"},
	}

	err := w.WriteFacts(context.Background(), facts, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gc.patchedProps) != 0 {
		t.Errorf("expected no patches, got %d", len(gc.patchedProps))
	}
}
