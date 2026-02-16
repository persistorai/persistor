package graphql

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// NodeService defines node operations needed by GraphQL resolvers.
type NodeService interface {
	ListNodes(ctx context.Context, tenantID string, typeFilter string, minSalience float64, limit, offset int) ([]models.Node, bool, error)
	GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	CreateNode(ctx context.Context, tenantID string, req models.CreateNodeRequest) (*models.Node, error)
	UpdateNode(ctx context.Context, tenantID string, nodeID string, req models.UpdateNodeRequest) (*models.Node, error)
	DeleteNode(ctx context.Context, tenantID, nodeID string) error
}

// EdgeStore defines edge operations needed by GraphQL resolvers.
type EdgeStore interface {
	ListEdges(ctx context.Context, tenantID string, source, target, relation string, limit, offset int) ([]models.Edge, bool, error)
	CreateEdge(ctx context.Context, tenantID string, req models.CreateEdgeRequest) (*models.Edge, error)
	UpdateEdge(ctx context.Context, tenantID string, source, target, relation string, req models.UpdateEdgeRequest) (*models.Edge, error)
	DeleteEdge(ctx context.Context, tenantID string, source, target, relation string) error
}

// SearchService defines search operations needed by GraphQL resolvers.
type SearchService interface {
	FullTextSearch(ctx context.Context, tenantID string, query string, typeFilter string, minSalience float64, limit int) ([]models.Node, error)
	SemanticSearch(ctx context.Context, tenantID, query string, limit int) ([]models.ScoredNode, error)
	HybridSearch(ctx context.Context, tenantID, query string, limit int) ([]models.Node, error)
}

// GraphStore defines graph traversal operations needed by GraphQL resolvers.
type GraphStore interface {
	Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	Traverse(ctx context.Context, tenantID string, nodeID string, maxHops int) (*models.TraverseResult, error)
	GraphContext(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error)
	ShortestPath(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error)
}

// SalienceStore defines salience operations needed by GraphQL resolvers.
type SalienceStore interface {
	BoostNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	SupersedeNode(ctx context.Context, tenantID, oldID, newID string) error
	RecalculateSalience(ctx context.Context, tenantID string) (int, error)
}

// AuditStore defines audit operations needed by GraphQL resolvers.
type AuditStore interface {
	QueryAudit(ctx context.Context, tenantID string, opts models.AuditQueryOpts) ([]models.AuditEntry, bool, error)
}

// Resolver is the root resolver for the GraphQL API.
// Field names use "Svc"/"Store" suffixes to avoid collision with generated resolver methods.
type Resolver struct {
	NodeSvc     NodeService
	EdgeStore   EdgeStore
	SearchSvc   SearchService
	GraphStore  GraphStore
	SalienceSvc SalienceStore
	AuditStore  AuditStore
}
