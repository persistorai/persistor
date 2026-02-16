package api

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// NodeRepository defines node operations used by NodeHandler.
type NodeRepository interface {
	ListNodes(ctx context.Context, tenantID string, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error)
	GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	CreateNode(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error)
	UpdateNode(ctx context.Context, tenantID string, nodeID string, req models.UpdateNodeRequest) (*models.Node, error)
	DeleteNode(ctx context.Context, tenantID, nodeID string) error
}

// EdgeRepository defines edge operations used by EdgeHandler.
type EdgeRepository interface {
	ListEdges(ctx context.Context, tenantID string, source, target, relation string, limit, offset int) ([]models.Edge, bool, error)
	CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	UpdateEdge(ctx context.Context, tenantID string, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	DeleteEdge(ctx context.Context, tenantID string, source, target, relation string) error
}

// SearchRepository defines search operations used by SearchHandler.
// The service layer handles embedding generation — handlers pass query strings.
type SearchRepository interface {
	FullTextSearch(ctx context.Context, tenantID string, query string, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	SemanticSearch(ctx context.Context, tenantID, query string, limit int) ([]models.ScoredNode, error)
	HybridSearch(ctx context.Context, tenantID, query string, limit int) ([]models.Node, error)
}

// GraphRepository defines graph traversal operations used by GraphHandler.
type GraphRepository interface {
	Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	Traverse(ctx context.Context, tenantID string, nodeID string, maxHops int) (*models.TraverseResult, error)
	GraphContext(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error)
	ShortestPath(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error)
}

// SalienceRepository defines salience operations used by SalienceHandler.
type SalienceRepository interface {
	BoostNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	SupersedeNode(ctx context.Context, tenantID, oldID, newID string) error
	RecalculateSalience(ctx context.Context, tenantID string) (int, error)
}

// BulkRepository defines bulk operations used by BulkHandler.
type BulkRepository interface {
	BulkUpsertNodes(ctx context.Context, tenantID string, nodes []models.CreateNodeRequest) (int, error)
	BulkUpsertEdges(ctx context.Context, tenantID string, edges []models.CreateEdgeRequest) (int, error)
}

// AuditRepository defines audit log operations used by AuditHandler.
type AuditRepository interface {
	Auditor
	QueryAudit(ctx context.Context, tenantID string, opts models.AuditQueryOpts) ([]models.AuditEntry, bool, error)
	PurgeOldEntries(ctx context.Context, tenantID string, retentionDays int) (int, error)
}

// Auditor is the minimal interface for recording audit entries.
// Services and handlers use this for fire-and-forget audit logging.
type Auditor interface {
	RecordAudit(ctx context.Context, tenantID, action, entityType, entityID, actor string, detail map[string]any) error
}
