// Package service provides business logic between API handlers and data stores.
package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// NodeStore is the data-access interface NodeService depends on.
// It reuses domain.NodeService since the method sets are identical, avoiding duplication.
type NodeStore = domain.NodeService

// Compile-time check: *NodeService must satisfy domain.NodeService.
var _ domain.NodeService = (*NodeService)(nil)

// EmbedEnqueuer enqueues embedding generation jobs.
type EmbedEnqueuer interface {
	Enqueue(job EmbedJob)
}

// Auditor is an alias for the canonical domain.Auditor interface.
type Auditor = domain.Auditor

// NodeService wraps NodeStore with business logic (embedding on create/update).
type NodeService struct {
	store       NodeStore
	embedWorker EmbedEnqueuer
	auditWorker AuditEnqueuer
	log         *logrus.Logger
}

// NewNodeService creates a NodeService.
func NewNodeService(store NodeStore, embedWorker EmbedEnqueuer, auditWorker AuditEnqueuer, log *logrus.Logger) *NodeService {
	return &NodeService{store: store, embedWorker: embedWorker, auditWorker: auditWorker, log: log}
}

// ListNodes returns a paginated list of nodes (pass-through).
func (s *NodeService) ListNodes(
	ctx context.Context, tenantID, typeFilter string, minSalience float64, limit, offset int,
) ([]models.Node, bool, error) {
	return s.store.ListNodes(ctx, tenantID, typeFilter, minSalience, limit, offset)
}

// GetNode returns a single node by ID (pass-through).
func (s *NodeService) GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	return s.store.GetNode(ctx, tenantID, nodeID)
}

// CreateNode creates a node and enqueues an embedding job.
func (s *NodeService) CreateNode(
	ctx context.Context, tenantID string, req models.CreateNodeRequest,
) (*models.Node, error) {
	node, err := s.store.CreateNode(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	if s.embedWorker != nil {
		s.embedWorker.Enqueue(EmbedJob{
			TenantID: tenantID,
			NodeID:   node.ID,
			Text:     node.Type + ":" + node.Label,
		})
	}

	auditAsync(s.auditWorker, tenantID, "node.create", "node", node.ID, map[string]any{"type": node.Type, "label": node.Label})

	return node, nil
}

// UpdateNode updates a node and re-embeds if type or label changed.
func (s *NodeService) UpdateNode(
	ctx context.Context, tenantID, nodeID string, req models.UpdateNodeRequest,
) (*models.Node, error) {
	node, err := s.store.UpdateNode(ctx, tenantID, nodeID, req)
	if err != nil {
		return nil, err
	}

	if req.Type != nil || req.Label != nil {
		if s.embedWorker != nil {
			s.embedWorker.Enqueue(EmbedJob{
				TenantID: tenantID,
				NodeID:   node.ID,
				Text:     node.Type + ":" + node.Label,
			})
		}
	}

	auditAsync(s.auditWorker, tenantID, "node.update", "node", node.ID, map[string]any{"type": node.Type, "label": node.Label})

	return node, nil
}

// PatchNodeProperties partially updates node properties (merge semantics).
func (s *NodeService) PatchNodeProperties(
	ctx context.Context, tenantID, nodeID string, req models.PatchPropertiesRequest,
) (*models.Node, error) {
	node, err := s.store.PatchNodeProperties(ctx, tenantID, nodeID, req)
	if err != nil {
		return nil, err
	}

	auditAsync(s.auditWorker, tenantID, "node.patch_properties", "node", nodeID, map[string]any{"patched_keys": mapKeys(req.Properties)})

	return node, nil
}

// mapKeys returns the keys of a map as a slice.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// MigrateNode atomically migrates a node to a new ID.
func (s *NodeService) MigrateNode(
	ctx context.Context, tenantID, oldID string, req models.MigrateNodeRequest,
) (*models.MigrateNodeResult, error) {
	result, err := s.store.MigrateNode(ctx, tenantID, oldID, req)
	if err != nil {
		return nil, err
	}

	// Re-embed the new node.
	if s.embedWorker != nil {
		label := result.NewID
		s.embedWorker.Enqueue(EmbedJob{TenantID: tenantID, NodeID: result.NewID, Text: label})
	}

	auditAsync(s.auditWorker, tenantID, "node.migrate", "node", oldID, map[string]any{
		"new_id":         result.NewID,
		"outgoing_edges": result.OutgoingEdges,
		"incoming_edges": result.IncomingEdges,
		"old_deleted":    result.OldDeleted,
	})

	return result, nil
}

// DeleteNode removes a node (pass-through).
func (s *NodeService) DeleteNode(ctx context.Context, tenantID, nodeID string) error {
	err := s.store.DeleteNode(ctx, tenantID, nodeID)
	if err == nil {
		auditAsync(s.auditWorker, tenantID, "node.delete", "node", nodeID, nil)
	}
	return err
}
