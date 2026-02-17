// Package domain defines the canonical service interfaces shared across API
// layers (REST, GraphQL, client). Consumers should depend on these interfaces
// rather than re-declaring equivalent ones.
package domain

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// NodeService defines all node operations.
type NodeService interface {
	ListNodes(ctx context.Context, tenantID string, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error)
	GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	CreateNode(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error)
	UpdateNode(ctx context.Context, tenantID string, nodeID string, req models.UpdateNodeRequest) (*models.Node, error)
	PatchNodeProperties(ctx context.Context, tenantID string, nodeID string, req models.PatchPropertiesRequest) (*models.Node, error)
	DeleteNode(ctx context.Context, tenantID, nodeID string) error
	MigrateNode(ctx context.Context, tenantID, oldID string, req models.MigrateNodeRequest) (*models.MigrateNodeResult, error)
}

// EdgeService defines all edge operations.
type EdgeService interface {
	ListEdges(ctx context.Context, tenantID string, source, target, relation string, limit, offset int) ([]models.Edge, bool, error)
	CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	UpdateEdge(ctx context.Context, tenantID string, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	PatchEdgeProperties(ctx context.Context, tenantID string, source, target, relation string, req models.PatchPropertiesRequest) (*models.Edge, error)
	DeleteEdge(ctx context.Context, tenantID string, source, target, relation string) error
}

// SearchService defines search operations.
// The service layer handles embedding generation — callers pass query strings.
type SearchService interface {
	FullTextSearch(ctx context.Context, tenantID string, query string, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	SemanticSearch(ctx context.Context, tenantID, query string, limit int) ([]models.ScoredNode, error)
	HybridSearch(ctx context.Context, tenantID, query string, limit int) ([]models.Node, error)
}

// GraphService defines graph traversal operations.
type GraphService interface {
	Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	Traverse(ctx context.Context, tenantID string, nodeID string, maxHops int) (*models.TraverseResult, error)
	GraphContext(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error)
	ShortestPath(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error)
}

// SalienceService defines salience scoring operations.
type SalienceService interface {
	BoostNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	SupersedeNode(ctx context.Context, tenantID, oldID, newID string) error
	RecalculateSalience(ctx context.Context, tenantID string) (int, error)
}

// BulkService defines bulk upsert operations.
type BulkService interface {
	BulkUpsertNodes(ctx context.Context, tenantID string, nodes []models.CreateNodeRequest) ([]models.Node, error)
	BulkUpsertEdges(ctx context.Context, tenantID string, edges []models.CreateEdgeRequest) ([]models.Edge, error)
}

// AuditService defines audit log query and maintenance operations.
type AuditService interface {
	Auditor
	QueryAudit(ctx context.Context, tenantID string, opts models.AuditQueryOpts) ([]models.AuditEntry, bool, error)
	PurgeOldEntries(ctx context.Context, tenantID string, retentionDays int) (int, error)
}

// Auditor is the minimal interface for recording audit entries.
// Used by services and handlers for fire-and-forget audit logging.
type Auditor interface {
	RecordAudit(ctx context.Context, tenantID, action, entityType, entityID, actor string, detail map[string]any) error
}

// AdminService defines administrative operations.
type AdminService interface {
	ListNodesWithoutEmbeddings(ctx context.Context, tenantID string, limit int) ([]models.NodeSummary, error)
}

// HistoryService defines property history operations.
type HistoryService interface {
	GetPropertyHistory(ctx context.Context, tenantID, nodeID string, propertyKey string, limit, offset int) ([]models.PropertyChange, bool, error)
}
