package store

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// Traversal safety limits.
const (
	traverseNodeLimit = 500  // max nodes returned from traverse
	traverseEdgeLimit = 5000 // max edges returned from traverse
	bfsNeighborLimit  = 1000 // max edges per direction in app-level BFS
	maxTraverseHops   = 5    // caps BFS depth
	maxPathHops       = 10   // caps shortest-path search depth
)

func graphNodeExists(ctx context.Context, tx pgx.Tx, nodeID string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1)`, nodeID).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking node existence: %w", err)
	}

	return exists, nil
}

func requireGraphNodesExist(ctx context.Context, tx pgx.Tx, nodeIDs ...string) error {
	for _, nodeID := range nodeIDs {
		exists, err := graphNodeExists(ctx, tx, nodeID)
		if err != nil {
			return err
		}

		if !exists {
			return models.ErrNodeNotFound
		}
	}

	return nil
}

func bfsNeighborPairs(ctx context.Context, tx pgx.Tx, frontier []string) ([][2]string, error) {
	if len(frontier) == 0 {
		return nil, nil
	}

	edges := make([][2]string, 0, len(frontier)*4)
	neighborSQL := `(SELECT DISTINCT source, target FROM kg_edges
		WHERE source = $1 AND tenant_id = current_setting('app.tenant_id')::uuid ORDER BY source, target LIMIT ` + fmt.Sprintf("%d", bfsNeighborLimit) + `)
		UNION
		(SELECT DISTINCT source, target FROM kg_edges
		WHERE target = $1 AND tenant_id = current_setting('app.tenant_id')::uuid ORDER BY source, target LIMIT ` + fmt.Sprintf("%d", bfsNeighborLimit) + `)`

	for _, nodeID := range frontier {
		rows, err := tx.Query(ctx, neighborSQL, nodeID)
		if err != nil {
			return nil, fmt.Errorf("querying BFS neighbors for %q: %w", nodeID, err)
		}

		for rows.Next() {
			var source, target string
			if err := rows.Scan(&source, &target); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning BFS edge: %w", err)
			}

			edges = append(edges, [2]string{source, target})
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterating BFS edges: %w", err)
		}

		rows.Close()
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i][0] != edges[j][0] {
			return edges[i][0] < edges[j][0]
		}

		return edges[i][1] < edges[j][1]
	})

	return edges, nil
}

// Traverse performs application-level BFS from nodeID up to maxHops and returns the discovered subgraph.
func (s *GraphStore) Traverse( //nolint:funlen,gocyclo,cyclop,gocognit // BFS loop with neighbor expansion is inherently multi-step.
	ctx context.Context,
	tenantID string,
	nodeID string,
	maxHops int,
) (*models.TraverseResult, error) {
	if maxHops <= 0 {
		maxHops = 1
	}

	if maxHops > maxTraverseHops {
		maxHops = maxTraverseHops
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("traversing graph: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	if err := requireGraphNodesExist(ctx, tx, nodeID); err != nil {
		return nil, err
	}

	// Application-level BFS with global visited set.
	visited := map[string]bool{nodeID: true}
	frontier := []string{nodeID}

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		edges, err := bfsNeighborPairs(ctx, tx, frontier)
		if err != nil {
			return nil, fmt.Errorf("querying traverse neighbors at hop %d: %w", hop, err)
		}

		var nextFrontier []string

		for _, edge := range edges {
			source, target := edge[0], edge[1]
			for _, pair := range [][2]string{{source, target}, {target, source}} {
				from, to := pair[0], pair[1]
				if visited[from] && !visited[to] {
					if len(visited) >= traverseNodeLimit {
						break
					}

					visited[to] = true
					nextFrontier = append(nextFrontier, to)
				}
			}

			if len(visited) >= traverseNodeLimit {
				break
			}
		}

		if len(visited) >= traverseNodeLimit {
			break
		}

		frontier = nextFrontier
	}

	// Collect all discovered node IDs.
	ids := make([]string, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return &models.TraverseResult{
			Nodes: make([]models.Node, 0),
			Edges: make([]models.Edge, 0),
		}, nil
	}

	// Fetch all discovered nodes.
	nodeSQL := `SELECT ` + nodeColumns + ` FROM kg_nodes
		WHERE id = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY id LIMIT ` + fmt.Sprintf("%d", traverseNodeLimit)

	nodeRows, err := tx.Query(ctx, nodeSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("querying traverse nodes: %w", err)
	}
	defer nodeRows.Close()

	nodes, err := collectNodes(nodeRows)
	if err != nil {
		return nil, fmt.Errorf("collecting traverse nodes: %w", err)
	}

	// Fetch all edges between discovered nodes.
	edgeSQL := `SELECT ` + edgeColumns + `
		FROM kg_edges
		WHERE source = ANY($1) AND target = ANY($1)
			AND tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY source, target LIMIT ` + fmt.Sprintf("%d", traverseEdgeLimit)

	edgeRows, err := tx.Query(ctx, edgeSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("querying traverse edges: %w", err)
	}
	defer edgeRows.Close()

	edgeList := make([]models.Edge, 0, 32)

	for edgeRows.Next() {
		e, err := scanEdge(edgeRows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning traverse edge: %w", err)
		}

		edgeList = append(edgeList, *e)
	}

	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating traverse edges: %w", err)
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}

	if err := s.decryptEdges(ctx, tenantID, edgeList); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing traverse: %w", err)
	}

	return &models.TraverseResult{Nodes: nodes, Edges: edgeList}, nil
}
