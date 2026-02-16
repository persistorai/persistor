package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// Graph query limits.
const (
	maxGraphNodeFetch    = 1000 // caps nodes fetched in a single graph query
	defaultEdgesPerQuery = 100  // default edges per direction in neighbor queries
	maxEdgesPerQuery     = 1000 // caps edges per direction
)

// GraphStore handles graph traversal and context queries.
type GraphStore struct {
	Base
}

// NewGraphStore creates a GraphStore with the given shared base.
func NewGraphStore(base Base) *GraphStore {
	return &GraphStore{Base: base}
}

// Neighbors returns all nodes directly connected to nodeID and the edges between them.
func (s *GraphStore) Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error) { //nolint:gocognit,gocyclo,cyclop,funlen // existence check adds necessary complexity.
	if limit <= 0 {
		limit = defaultEdgesPerQuery
	}

	if limit > maxEdgesPerQuery {
		limit = maxEdgesPerQuery
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting neighbors: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Verify the root node exists before querying edges.
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1)`, nodeID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("checking node existence: %w", err)
	}

	if !exists {
		return nil, models.ErrNodeNotFound
	}

	// Rewrite OR as UNION ALL with per-direction limits.
	edgeSQL := `(SELECT ` + edgeColumns + `
		FROM kg_edges
		WHERE source = $1 AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT $2)
		UNION ALL
		(SELECT ` + edgeColumns + `
		FROM kg_edges
		WHERE target = $1 AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT $2)`

	edgeRows, err := tx.Query(ctx, edgeSQL, nodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying neighbor edges: %w", err)
	}
	defer edgeRows.Close()

	edgeList := make([]models.Edge, 0, 32)
	neighborIDs := make(map[string]bool)

	for edgeRows.Next() {
		e, err := scanEdge(edgeRows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning neighbor edge: %w", err)
		}

		edgeList = append(edgeList, *e)

		if e.Source != nodeID {
			neighborIDs[e.Source] = true
		}

		if e.Target != nodeID {
			neighborIDs[e.Target] = true
		}
	}

	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating neighbor edges: %w", err)
	}

	// Fetch all neighbor nodes in a single query.
	ids := make([]string, 0, len(neighborIDs))
	for nid := range neighborIDs {
		ids = append(ids, nid)
	}

	nodeList := make([]models.Node, 0, len(ids))

	if len(ids) > 0 {
		nodeSQL := `SELECT ` + nodeColumns + ` FROM kg_nodes WHERE id = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT ` + fmt.Sprintf("%d", maxGraphNodeFetch)

		nodeRows, err := tx.Query(ctx, nodeSQL, ids)
		if err != nil {
			return nil, fmt.Errorf("querying neighbor nodes: %w", err)
		}
		defer nodeRows.Close()

		nodeList, err = collectNodes(nodeRows)
		if err != nil {
			return nil, fmt.Errorf("collecting neighbor nodes: %w", err)
		}
	}

	if err := s.decryptNodes(ctx, tenantID, nodeList); err != nil {
		return nil, err
	}

	if err := s.decryptEdges(ctx, tenantID, edgeList); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing neighbors: %w", err)
	}

	return &models.NeighborResult{Nodes: nodeList, Edges: edgeList}, nil
}

// GraphContext returns a node with its immediate neighbors and connecting edges.
func (s *GraphStore) GraphContext( //nolint:gocognit,gocyclo,cyclop,funlen // inherent complexity from multi-query graph assembly.
	ctx context.Context,
	tenantID string,
	nodeID string,
) (*models.ContextResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting graph context: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Get the node itself.
	nodeSQL := `SELECT ` + nodeColumns + ` FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`
	row := tx.QueryRow(ctx, nodeSQL, nodeID)

	node, err := scanNode(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNodeNotFound
		}

		return nil, fmt.Errorf("scanning context node: %w", err)
	}

	// Get connecting edges using UNION ALL with per-direction limits.
	edgeSQL := `(SELECT ` + edgeColumns + `
		FROM kg_edges
		WHERE source = $1 AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT $2)
		UNION ALL
		(SELECT ` + edgeColumns + `
		FROM kg_edges
		WHERE target = $1 AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT $2)`

	edgeRows, err := tx.Query(ctx, edgeSQL, nodeID, maxEdgesPerQuery)
	if err != nil {
		return nil, fmt.Errorf("querying context edges: %w", err)
	}
	defer edgeRows.Close()

	edgeList := make([]models.Edge, 0, 32)
	neighborIDs := make(map[string]bool)

	for edgeRows.Next() {
		e, err := scanEdge(edgeRows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning context edge: %w", err)
		}

		edgeList = append(edgeList, *e)

		if e.Source != nodeID {
			neighborIDs[e.Source] = true
		}

		if e.Target != nodeID {
			neighborIDs[e.Target] = true
		}
	}

	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating context edges: %w", err)
	}

	// Fetch all neighbor nodes in a single query.
	ids := make([]string, 0, len(neighborIDs))
	for nid := range neighborIDs {
		ids = append(ids, nid)
	}

	neighbors := make([]models.Node, 0, len(ids))

	if len(ids) > 0 {
		nSQL := `SELECT ` + nodeColumns + ` FROM kg_nodes WHERE id = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT ` + fmt.Sprintf("%d", maxGraphNodeFetch)

		nRows, err := tx.Query(ctx, nSQL, ids)
		if err != nil {
			return nil, fmt.Errorf("querying context neighbors: %w", err)
		}
		defer nRows.Close()

		neighbors, err = collectNodes(nRows)
		if err != nil {
			return nil, fmt.Errorf("collecting context neighbors: %w", err)
		}
	}

	if err := s.decryptNode(ctx, tenantID, node); err != nil {
		return nil, err
	}

	if err := s.decryptNodes(ctx, tenantID, neighbors); err != nil {
		return nil, err
	}

	if err := s.decryptEdges(ctx, tenantID, edgeList); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing graph context: %w", err)
	}

	return &models.ContextResult{Node: *node, Neighbors: neighbors, Edges: edgeList}, nil
}
