package ingest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/persistorai/persistor/client"
)

type resolverMockGraphClient struct {
	idNodes      map[string]*client.Node
	labelNodes   map[string]*client.Node
	searchNodes  []client.Node
	createdNodes []client.CreateNodeRequest
	patchedProps map[string]map[string]any
	nodeIDSeq    int
}

func newResolverMockGraphClient() *resolverMockGraphClient {
	return &resolverMockGraphClient{
		idNodes:      make(map[string]*client.Node),
		labelNodes:   make(map[string]*client.Node),
		patchedProps: make(map[string]map[string]any),
	}
}

func (m *resolverMockGraphClient) GetNode(_ context.Context, id string) (*client.Node, error) {
	return m.idNodes[id], nil
}

func (m *resolverMockGraphClient) GetNodeByLabel(_ context.Context, label string) (*client.Node, error) {
	return m.labelNodes[label], nil
}

func (m *resolverMockGraphClient) SearchNodes(_ context.Context, _ string, _ int) ([]client.Node, error) {
	return m.searchNodes, nil
}

func (m *resolverMockGraphClient) CreateNode(_ context.Context, req *client.CreateNodeRequest) (*client.Node, error) {
	m.nodeIDSeq++
	m.createdNodes = append(m.createdNodes, *req)
	return &client.Node{ID: fmt.Sprintf("node-%d", m.nodeIDSeq), Label: req.Label, Type: req.Type}, nil
}

func (m *resolverMockGraphClient) UpdateNode(_ context.Context, id string, _ *client.UpdateNodeRequest) (*client.Node, error) {
	return &client.Node{ID: id}, nil
}

func (m *resolverMockGraphClient) PatchNodeProperties(_ context.Context, id string, props map[string]any) (*client.Node, error) {
	m.patchedProps[id] = props
	return &client.Node{ID: id}, nil
}

func (m *resolverMockGraphClient) CreateEdge(_ context.Context, req *client.CreateEdgeRequest) (*client.Edge, error) {
	return &client.Edge{Source: req.Source, Target: req.Target, Relation: req.Relation}, nil
}

func (m *resolverMockGraphClient) UpdateEdge(_ context.Context, source, target, relation string, _ *client.UpdateEdgeRequest) (*client.Edge, error) {
	return &client.Edge{Source: source, Target: target, Relation: relation}, nil
}

func TestResolveEntity_ExactIDWins(t *testing.T) {
	gc := newResolverMockGraphClient()
	gc.idNodes["node-123"] = &client.Node{ID: "node-123", Label: "Alice", Type: "person"}

	w := NewWriter(gc, "test-source")
	resolution, err := w.resolveEntity(context.Background(), "node-123", "person")
	if err != nil {
		t.Fatalf("resolveEntity: %v", err)
	}
	if resolution.Status != resolutionMatched {
		t.Fatalf("status = %q, want %q", resolution.Status, resolutionMatched)
	}
	if resolution.Match == nil || resolution.Match.Node == nil || resolution.Match.Node.ID != "node-123" {
		t.Fatalf("match = %#v, want node-123", resolution.Match)
	}
	if resolution.Match.Method != matchMethodExactID {
		t.Fatalf("method = %q, want %q", resolution.Match.Method, matchMethodExactID)
	}
}

func TestResolveEntity_AliasAwareExactLookup(t *testing.T) {
	gc := newResolverMockGraphClient()
	gc.labelNodes["IBM"] = &client.Node{ID: "org-1", Label: "International Business Machines", Type: "company"}

	w := NewWriter(gc, "test-source")
	resolution, err := w.resolveEntity(context.Background(), "IBM", "company")
	if err != nil {
		t.Fatalf("resolveEntity: %v", err)
	}
	if resolution.Status != resolutionMatched {
		t.Fatalf("status = %q, want %q", resolution.Status, resolutionMatched)
	}
	if resolution.Match == nil || resolution.Match.Node == nil || resolution.Match.Node.ID != "org-1" {
		t.Fatalf("match = %#v, want org-1", resolution.Match)
	}
	if resolution.Match.Method != matchMethodAlias {
		t.Fatalf("method = %q, want %q", resolution.Match.Method, matchMethodAlias)
	}
}

func TestResolveEntity_TypeAwareMatchAutoUpdates(t *testing.T) {
	gc := newResolverMockGraphClient()
	gc.searchNodes = []client.Node{{ID: "acme-1", Label: "Acme, Inc.", Type: "company", UpdatedAt: time.Now()}}

	w := NewWriter(gc, "test-source")
	report, nodeMap, err := w.WriteEntities(context.Background(), []ExtractedEntity{{Name: "Acme", Type: "company", Properties: map[string]any{"stage": "seed"}}})
	if err != nil {
		t.Fatalf("WriteEntities: %v", err)
	}
	if report.UpdatedNodes != 1 || report.CreatedNodes != 0 {
		t.Fatalf("report = %#v, want 1 update and 0 creates", report)
	}
	if nodeMap["acme"] != "acme-1" {
		t.Fatalf("nodeMap[acme] = %q, want acme-1", nodeMap["acme"])
	}
	if _, ok := gc.patchedProps["acme-1"]; !ok {
		t.Fatal("expected matched node to be patched")
	}
}

func TestResolveEntity_AmbiguousCandidatesDoNotAutoMatch(t *testing.T) {
	gc := newResolverMockGraphClient()
	now := time.Now()
	gc.searchNodes = []client.Node{
		{ID: "alpha-1", Label: "Alpha Labs", Type: "company", UpdatedAt: now},
		{ID: "alpha-2", Label: "Alpha Lab", Type: "company", UpdatedAt: now.Add(-time.Minute)},
	}

	w := NewWriter(gc, "test-source")
	resolution, err := w.resolveEntity(context.Background(), "Alpha Labz", "")
	if err != nil {
		t.Fatalf("resolveEntity: %v", err)
	}
	if resolution.Status != resolutionAmbiguous {
		t.Fatalf("status = %q, want %q with candidates %#v", resolution.Status, resolutionAmbiguous, resolution.Candidates)
	}
	if len(resolution.Candidates) < 2 {
		t.Fatalf("candidates = %d, want at least 2", len(resolution.Candidates))
	}

	report, nodeMap, err := w.WriteEntities(context.Background(), []ExtractedEntity{{Name: "Alpha Labz", Properties: map[string]any{"seen": true}}})
	if err != nil {
		t.Fatalf("WriteEntities: %v", err)
	}
	if report.CreatedNodes != 1 || report.UpdatedNodes != 0 {
		t.Fatalf("report = %#v, want create without silent merge", report)
	}
	if got := nodeMap["alpha labz"]; got != "node-1" {
		t.Fatalf("nodeMap[alpha labz] = %q, want created node-1", got)
	}
	if len(gc.patchedProps) != 0 {
		t.Fatalf("patchedProps = %#v, want no automatic patch on ambiguous match", gc.patchedProps)
	}
}

func TestResolveEntityInKG_AmbiguousMatchSkipsRelationshipResolution(t *testing.T) {
	gc := newResolverMockGraphClient()
	now := time.Now()
	gc.searchNodes = []client.Node{
		{ID: "alpha-1", Label: "Alpha Labs", Type: "company", UpdatedAt: now},
		{ID: "alpha-2", Label: "Alpha Lab", Type: "company", UpdatedAt: now.Add(-time.Minute)},
	}

	w := NewWriter(gc, "test-source")
	report, err := w.WriteRelationships(context.Background(), []ExtractedRelationship{{Source: "Alpha Labz", Target: "Known", Relation: "works_on", Confidence: 0.9}}, map[string]string{"known": "known-1"})
	if err != nil {
		t.Fatalf("WriteRelationships: %v", err)
	}
	if report.CreatedEdges != 0 || report.SkippedEdges != 1 {
		t.Fatalf("report = %#v, want skipped ambiguous edge", report)
	}
}

func TestTrigramSimilarity(t *testing.T) {
	if got := trigramSimilarity("john smith", "john smith"); got != 1 {
		t.Fatalf("exact trigram similarity = %f, want 1", got)
	}
	if got := trigramSimilarity("john smith", "jon smith"); got <= 0.72 {
		t.Fatalf("close trigram similarity = %f, want > 0.72", got)
	}
	if got := trigramSimilarity("john smith", "acme corp"); got != 0 {
		t.Fatalf("distant trigram similarity = %f, want 0", got)
	}
}
