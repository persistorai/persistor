// Package service implements business logic for the knowledge graph.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/persistorai/persistor/internal/db"
	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// exportImportStore is the minimal store interface consumed by ExportImportService.
// Defined at the consumer (per project convention) so the store package depends
// on no service types.
type exportImportStore interface {
	ExportAllNodes(ctx context.Context, tenantID string) ([]models.ExportNode, error)
	ExportAllEdges(ctx context.Context, tenantID string) ([]models.ExportEdge, error)
	UpsertNodeFromExport(ctx context.Context, tenantID string, node models.ExportNode, overwrite bool) (string, error)
	UpsertEdgeFromExport(ctx context.Context, tenantID string, edge models.ExportEdge, overwrite bool) (string, error)
}

// Compile-time check: *ExportImportService must satisfy domain.ExportImportService.
var _ domain.ExportImportService = (*ExportImportService)(nil)

// ExportImportService implements domain.ExportImportService.
type ExportImportService struct {
	store            exportImportStore
	persistorVersion string
}

// NewExportImportService creates an ExportImportService.
func NewExportImportService(store exportImportStore, persistorVersion string) *ExportImportService {
	return &ExportImportService{store: store, persistorVersion: persistorVersion}
}

// Export serialises all nodes and edges for a tenant into a portable, full-fidelity format.
// Properties are returned in plaintext; the store layer handles decryption.
func (s *ExportImportService) Export(ctx context.Context, tenantID string) (*models.ExportFormat, error) {
	nodes, err := s.store.ExportAllNodes(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("exporting nodes: %w", err)
	}

	edges, err := s.store.ExportAllEdges(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("exporting edges: %w", err)
	}

	return &models.ExportFormat{
		SchemaVersion:    db.SchemaVersion(),
		PersistorVersion: s.persistorVersion,
		ExportedAt:       time.Now().UTC(),
		TenantID:         tenantID,
		Stats: models.ExportStats{
			NodeCount: len(nodes),
			EdgeCount: len(edges),
		},
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// ValidateImport checks an export payload for consistency errors without writing
// anything to the database. Returns a list of human-readable error descriptions.
// An empty slice means the payload is valid.
func (s *ExportImportService) ValidateImport(ctx context.Context, tenantID string, data *models.ExportFormat) ([]string, error) {
	current := db.SchemaVersion()

	var errs []string

	if data.SchemaVersion > current {
		errs = append(errs, fmt.Sprintf(
			"export schema version %d is newer than this instance (%d); upgrade Persistor before importing",
			data.SchemaVersion, current,
		))
	}

	errs = append(errs, validateNodes(data.Nodes)...)

	dbNodeIDs, err := s.fetchDBNodeIDs(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("fetching existing node IDs for validation: %w", err)
	}

	exportNodeIDs := buildNodeIDSet(data.Nodes)
	errs = append(errs, validateEdges(data.Edges, exportNodeIDs, dbNodeIDs)...)

	return errs, nil
}

// Import ingests a previously exported payload into the tenant's graph.
// Nodes are imported before edges because edges reference nodes.
func (s *ExportImportService) Import(
	ctx context.Context,
	tenantID string,
	data *models.ExportFormat,
	opts models.ImportOptions,
) (*models.ImportResult, error) {
	if data.SchemaVersion > db.SchemaVersion() {
		return nil, fmt.Errorf("export was created by a newer version of Persistor")
	}

	errs, err := s.ValidateImport(ctx, tenantID, data)
	if err != nil {
		return nil, fmt.Errorf("validating import: %w", err)
	}

	if len(errs) > 0 {
		return &models.ImportResult{Errors: errs}, nil
	}

	result := &models.ImportResult{}

	if opts.DryRun {
		result.NodesCreated = len(data.Nodes)
		result.EdgesCreated = len(data.Edges)
		return result, nil
	}

	if err := s.importNodes(ctx, tenantID, data.Nodes, opts, result); err != nil {
		return nil, err
	}

	if err := s.importEdges(ctx, tenantID, data.Edges, opts, result); err != nil {
		return nil, err
	}

	return result, nil
}

// importNodes upserts all nodes from the export and updates result counts.
func (s *ExportImportService) importNodes(
	ctx context.Context,
	tenantID string,
	nodes []models.ExportNode,
	opts models.ImportOptions,
	result *models.ImportResult,
) error {
	for _, n := range nodes {
		n = applyNodeOptions(n, opts)

		action, err := s.store.UpsertNodeFromExport(ctx, tenantID, n, opts.OverwriteExisting)
		if err != nil {
			return fmt.Errorf("importing node %s: %w", n.ID, err)
		}

		switch action {
		case "created":
			result.NodesCreated++
		case "updated":
			result.NodesUpdated++
		case "skipped":
			result.NodesSkipped++
		}
	}

	return nil
}

// importEdges upserts all edges from the export and updates result counts.
func (s *ExportImportService) importEdges(
	ctx context.Context,
	tenantID string,
	edges []models.ExportEdge,
	opts models.ImportOptions,
	result *models.ImportResult,
) error {
	for _, e := range edges {
		e = applyEdgeOptions(e, opts)

		action, err := s.store.UpsertEdgeFromExport(ctx, tenantID, e, opts.OverwriteExisting)
		if err != nil {
			return fmt.Errorf("importing edge %s→%s (%s): %w", e.Source, e.Target, e.Relation, err)
		}

		switch action {
		case "created":
			result.EdgesCreated++
		case "updated":
			result.EdgesUpdated++
		case "skipped":
			result.EdgesSkipped++
		}
	}

	return nil
}

// fetchDBNodeIDs returns the set of node IDs that already exist in the DB for
// a tenant. Used by ValidateImport to check that edge endpoints are resolvable.
func (s *ExportImportService) fetchDBNodeIDs(ctx context.Context, tenantID string) (map[string]struct{}, error) {
	existing, err := s.store.ExportAllNodes(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		ids[n.ID] = struct{}{}
	}

	return ids, nil
}

// applyNodeOptions applies ImportOptions to a node before storing it.
func applyNodeOptions(n models.ExportNode, opts models.ImportOptions) models.ExportNode {
	if opts.ResetUsage {
		n.AccessCount = 0
		n.LastAccessed = nil
	}

	if opts.RegenerateEmbeddings {
		n.Embedding = nil
	}

	return n
}

// applyEdgeOptions applies ImportOptions to an edge before storing it.
func applyEdgeOptions(e models.ExportEdge, opts models.ImportOptions) models.ExportEdge {
	if opts.ResetUsage {
		e.AccessCount = 0
		e.LastAccessed = nil
	}

	return e
}

// buildNodeIDSet builds a set of node IDs from an export node slice.
func buildNodeIDSet(nodes []models.ExportNode) map[string]struct{} {
	ids := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = struct{}{}
	}

	return ids
}

// validateNodes checks that every node has a non-empty ID.
func validateNodes(nodes []models.ExportNode) []string {
	var errs []string

	for i, n := range nodes {
		if n.ID == "" {
			errs = append(errs, fmt.Sprintf("node[%d] has an empty ID", i))
		}
	}

	return errs
}

// validateEdges checks that every edge's source and target IDs resolve to a
// known node — either in the export payload or already present in the DB.
func validateEdges(edges []models.ExportEdge, exportIDs, dbIDs map[string]struct{}) []string {
	var errs []string

	for i, e := range edges {
		if _, inExport := exportIDs[e.Source]; !inExport {
			if _, inDB := dbIDs[e.Source]; !inDB {
				errs = append(errs, fmt.Sprintf(
					"edge[%d] source %q not found in export data or database", i, e.Source,
				))
			}
		}

		if _, inExport := exportIDs[e.Target]; !inExport {
			if _, inDB := dbIDs[e.Target]; !inDB {
				errs = append(errs, fmt.Sprintf(
					"edge[%d] target %q not found in export data or database", i, e.Target,
				))
			}
		}
	}

	return errs
}
