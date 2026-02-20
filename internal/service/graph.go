// Package service provides business logic between API handlers and data stores.
package service

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/domain"
	"github.com/persistorai/persistor/internal/models"
)

// GraphStore is the data-access interface GraphService depends on.
// It reuses domain.GraphService since the method sets are identical, avoiding duplication.
type GraphStore = domain.GraphService

// Compile-time check: *GraphService must satisfy domain.GraphService.
var _ domain.GraphService = (*GraphService)(nil)

// GraphService wraps GraphStore with context-aware logging.
type GraphService struct {
	store GraphStore
	log   *logrus.Logger
}

// NewGraphService creates a GraphService.
func NewGraphService(store GraphStore, log *logrus.Logger) *GraphService {
	return &GraphService{store: store, log: log}
}

// Neighbors returns all nodes directly connected to nodeID.
func (s *GraphService) Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"node_id":   nodeID,
		"limit":     limit,
	}).Debug("graph.neighbors")

	return s.store.Neighbors(ctx, tenantID, nodeID, limit)
}

// Traverse performs a multi-hop graph traversal starting from nodeID.
func (s *GraphService) Traverse(ctx context.Context, tenantID, nodeID string, maxHops int) (*models.TraverseResult, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"node_id":   nodeID,
		"max_hops":  maxHops,
	}).Debug("graph.traverse")

	return s.store.Traverse(ctx, tenantID, nodeID, maxHops)
}

// GraphContext returns a node with its immediate neighbors and connecting edges.
func (s *GraphService) GraphContext(ctx context.Context, tenantID, nodeID string) (*models.ContextResult, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"node_id":   nodeID,
	}).Debug("graph.context")

	return s.store.GraphContext(ctx, tenantID, nodeID)
}

// ShortestPath finds the shortest path between two nodes.
func (s *GraphService) ShortestPath(ctx context.Context, tenantID, fromID, toID string) ([]models.Node, error) {
	s.log.WithFields(logrus.Fields{
		"tenant_id": tenantID,
		"from_id":   fromID,
		"to_id":     toID,
	}).Debug("graph.shortest_path")

	return s.store.ShortestPath(ctx, tenantID, fromID, toID)
}
