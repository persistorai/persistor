package ingest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/persistorai/persistor/client"
	"github.com/persistorai/persistor/internal/ingest"
	"github.com/persistorai/persistor/internal/models"
)

// mockGraphClient implements ingest.GraphClient for testing.
type mockGraphClient struct {
	idNodes      map[string]*client.Node
	labelNodes   map[string]*client.Node
	searchNodes  []client.Node
	createdNodes []client.CreateNodeRequest
	patchedProps map[string]map[string]any
	createdEdges []client.CreateEdgeRequest
	nodeIDSeq    int
	createErr    error
}

type mockEpisodicClient struct {
	episodes []models.CreateEpisodeRequest
	events   []models.CreateEventRecordRequest
}

func newMockGraphClient() *mockGraphClient {
	return &mockGraphClient{
		idNodes:      make(map[string]*client.Node),
		labelNodes:   make(map[string]*client.Node),
		patchedProps: make(map[string]map[string]any),
	}
}

func (m *mockGraphClient) GetNode(_ context.Context, id string) (*client.Node, error) {
	return m.idNodes[id], nil
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

func (m *mockGraphClient) UpdateEdge(_ context.Context, source, target, relation string, _ *client.UpdateEdgeRequest) (*client.Edge, error) {
	return &client.Edge{Source: source, Target: target, Relation: relation}, nil
}

func (m *mockEpisodicClient) CreateEpisode(_ context.Context, _ string, req models.CreateEpisodeRequest) (*models.Episode, error) {
	if req.ID == "" {
		req.ID = fmt.Sprintf("episode-%d", len(m.episodes)+1)
	}
	m.episodes = append(m.episodes, req)
	return &models.Episode{
		ID:         req.ID,
		Title:      req.Title,
		Summary:    req.Summary,
		StartedAt:  req.StartedAt,
		EndedAt:    req.EndedAt,
		Properties: req.Properties,
	}, nil
}

func (m *mockEpisodicClient) CreateEventRecord(_ context.Context, _ string, req models.CreateEventRecordRequest) (*models.EventRecord, error) {
	if req.ID == "" {
		req.ID = fmt.Sprintf("event-%d", len(m.events)+1)
	}
	m.events = append(m.events, req)
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	return &models.EventRecord{
		ID:              req.ID,
		EpisodeID:       req.EpisodeID,
		Kind:            req.Kind,
		Title:           req.Title,
		Summary:         req.Summary,
		OccurredAt:      req.OccurredAt,
		OccurredStartAt: req.OccurredStartAt,
		OccurredEndAt:   req.OccurredEndAt,
		Confidence:      confidence,
		Evidence:        req.Evidence,
		Properties:      req.Properties,
	}, nil
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

	entities := []ingest.ExtractedEntity{{Name: "Alice", Type: "person", Properties: map[string]any{"role": "dev"}}}

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
	rels := []ingest.ExtractedRelationship{{Source: "Alice", Target: "Persistor", Relation: "works_on", Confidence: 0.9}}

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
	rels := []ingest.ExtractedRelationship{{Source: "Alice", Target: "Bob", Relation: "invented_by", Confidence: 0.8}}

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
	rels := []ingest.ExtractedRelationship{{Source: "Alice", Target: "Unknown", Relation: "works_on", Confidence: 0.9}}

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
	facts := []ingest.ExtractedFact{{Subject: "Alice", Property: "age", Value: "30"}}

	err := w.WriteFacts(context.Background(), facts, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patched, ok := gc.patchedProps["id-1"]
	if !ok {
		t.Fatal("expected properties to be patched on id-1")
	}

	rawUpdates, ok := patched[models.FactUpdatesProperty]
	if !ok {
		t.Fatalf("expected %s payload, got %#v", models.FactUpdatesProperty, patched)
	}
	updates, ok := rawUpdates.(map[string]models.FactUpdate)
	if !ok {
		t.Fatalf("fact updates type = %T, want map[string]models.FactUpdate", rawUpdates)
	}
	update := updates["age"]
	if update.Value != "30" {
		t.Errorf("expected age=30, got %v", update.Value)
	}
	if update.Source != "test-source" {
		t.Errorf("expected source=test-source, got %q", update.Source)
	}
}

func TestWriteFacts_UnknownSubjectSkipped(t *testing.T) {
	gc := newMockGraphClient()
	w := ingest.NewWriter(gc, "test-source")

	nodeMap := map[string]string{}
	facts := []ingest.ExtractedFact{{Subject: "Unknown", Property: "age", Value: "30"}}

	err := w.WriteFacts(context.Background(), facts, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gc.patchedProps) != 0 {
		t.Errorf("expected no patches, got %d", len(gc.patchedProps))
	}
}

func TestWriteEpisodic_CreatesEpisodeAndLinkedEvents(t *testing.T) {
	gc := newMockGraphClient()
	ep := &mockEpisodicClient{}
	w := ingest.NewWriter(gc, "notes/roadmap.md").WithEpisodic(ep, "tenant-1")

	entities := []ingest.ExtractedEntity{
		{Name: "Launch Plan", Type: "event", Description: "Team reviewed the launch checklist.", Properties: map[string]any{"date": "2026-04-01"}},
		{Name: "Ship v2", Type: "decision", Description: "Brian chose to ship v2 this week.", Properties: map[string]any{"occurred_at": "2026-04-02T10:00:00Z"}},
		{Name: "Follow-up", Type: "event", Description: "Promise to send the rollout note.", Properties: map[string]any{"event_kind": "promise"}},
	}
	nodeMap := map[string]string{"launch plan": "node-1", "ship v2": "node-2", "follow-up": "node-3", "brian": "node-4", "persistor": "node-5"}
	rels := []ingest.ExtractedRelationship{{Source: "Brian", Target: "Persistor", Relation: "decided", Confidence: 0.91, DateStart: strPtr("2026-04-03")}}

	createdEpisodes, createdEvents, err := w.WriteEpisodic(context.Background(), entities, rels, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdEpisodes != 1 {
		t.Fatalf("createdEpisodes = %d, want 1", createdEpisodes)
	}
	if createdEvents != 4 {
		t.Fatalf("createdEvents = %d, want 4", createdEvents)
	}
	if len(ep.episodes) != 1 {
		t.Fatalf("episodes persisted = %d, want 1", len(ep.episodes))
	}
	if got := ep.episodes[0].Title; got != "Ingested episode: roadmap.md" {
		t.Fatalf("episode title = %q", got)
	}
	if len(ep.events) != 4 {
		t.Fatalf("events persisted = %d, want 4", len(ep.events))
	}
	if ep.events[0].EpisodeID == nil || *ep.events[0].EpisodeID == "" {
		t.Fatal("expected event episode ID to be assigned")
	}
	if got := ep.events[1].Kind; got != models.EventKindDecision {
		t.Fatalf("entity decision kind = %q, want decision", got)
	}
	if got := ep.events[2].Kind; got != models.EventKindDecision {
		t.Fatalf("relationship-derived kind = %q, want decision", got)
	}
	if got := ep.events[3].Kind; got != models.EventKindPromise {
		t.Fatalf("explicit promise kind = %q, want promise", got)
	}
	decisionLinks := ep.events[2].Links
	if len(decisionLinks) != 2 {
		t.Fatalf("relationship-derived links = %d, want 2", len(decisionLinks))
	}
	if ep.episodes[0].StartedAt == nil || ep.episodes[0].EndedAt == nil {
		t.Fatal("expected episode time bounds to be set")
	}
}

func strPtr(v string) *string {
	return &v
}
