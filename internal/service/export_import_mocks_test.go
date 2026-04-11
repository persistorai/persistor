package service_test

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/service"
)

// mockExportImportStore implements service.exportImportStore for tests.
type mockExportImportStore struct {
	nodes                []models.ExportNode
	edges                []models.ExportEdge
	errOnExport          error
	errOnExistingNodeIDs error
	upsertErr            error
	existingNodeIDsCalls int
	lastExistingNodeIDs  []string
}

func (m *mockExportImportStore) ExportAllNodes(_ context.Context, _ string) ([]models.ExportNode, error) {
	if m.errOnExport != nil {
		return nil, m.errOnExport
	}
	return m.nodes, nil
}

func (m *mockExportImportStore) ExportAllEdges(_ context.Context, _ string) ([]models.ExportEdge, error) {
	if m.errOnExport != nil {
		return nil, m.errOnExport
	}
	return m.edges, nil
}

func (m *mockExportImportStore) ExistingNodeIDs(_ context.Context, _ string, ids []string) (map[string]struct{}, error) {
	m.existingNodeIDsCalls++
	m.lastExistingNodeIDs = append([]string(nil), ids...)
	if m.errOnExistingNodeIDs != nil {
		return nil, m.errOnExistingNodeIDs
	}

	result := make(map[string]struct{})
	existing := make(map[string]struct{}, len(m.nodes))
	for _, node := range m.nodes {
		existing[node.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := existing[id]; ok {
			result[id] = struct{}{}
		}
	}

	return result, nil
}

func (m *mockExportImportStore) UpsertNodeFromExport(_ context.Context, _ string, _ models.ExportNode, _ bool) (string, error) {
	if m.upsertErr != nil {
		return "", m.upsertErr
	}
	return "created", nil
}

func (m *mockExportImportStore) UpsertEdgeFromExport(_ context.Context, _ string, _ models.ExportEdge, _ bool) (string, error) {
	if m.upsertErr != nil {
		return "", m.upsertErr
	}
	return "created", nil
}

func newTestService(store *mockExportImportStore) *service.ExportImportService {
	return service.NewExportImportService(store, "test-0.0.1")
}
