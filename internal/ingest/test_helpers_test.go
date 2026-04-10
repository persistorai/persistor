package ingest

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/client"
)

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
