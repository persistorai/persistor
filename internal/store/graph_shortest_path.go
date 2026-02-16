package store

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/persistorai/persistor/internal/models"
)

// ShortestPath finds the shortest path between two nodes using application-level BFS.
// Returns an ordered slice of nodes from fromID to toID, or nil if no path exists.
func (s *GraphStore) ShortestPath( //nolint:gocognit,gocyclo,cyclop,funlen // BFS loop with parent tracking is inherently multi-step.
	ctx context.Context,
	tenantID, fromID, toID string,
) ([]models.Node, error) {
	if fromID == toID {
		return s.fetchPathNodes(ctx, tenantID, []string{fromID})
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("finding shortest path: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// BFS safety caps.
	const maxVisitedNodes = 10000
	const maxFrontierPerHop = 500

	visited := map[string]bool{fromID: true}
	parent := map[string]string{} // child -> parent
	frontier := []string{fromID}

	neighborSQL := `(SELECT DISTINCT source, target FROM kg_edges
		WHERE source = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT ` + fmt.Sprintf("%d", bfsNeighborLimit) + `)
		UNION
		(SELECT DISTINCT source, target FROM kg_edges
		WHERE target = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT ` + fmt.Sprintf("%d", bfsNeighborLimit) + `)`

	found := false

	for hop := 0; hop < maxPathHops && !found && len(frontier) > 0; hop++ {
		if len(visited) >= maxVisitedNodes {
			break
		}

		rows, err := tx.Query(ctx, neighborSQL, frontier)
		if err != nil {
			return nil, fmt.Errorf("querying BFS neighbors at hop %d: %w", hop, err)
		}

		var nextFrontier []string

		for rows.Next() {
			var source, target string
			if err := rows.Scan(&source, &target); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning BFS edge: %w", err)
			}

			for _, pair := range [][2]string{{source, target}, {target, source}} {
				from, to := pair[0], pair[1]
				if visited[from] && !visited[to] {
					visited[to] = true
					parent[to] = from
					nextFrontier = append(nextFrontier, to)

					if to == toID {
						found = true
					}
				}
			}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterating BFS edges: %w", err)
		}

		rows.Close()

		if len(nextFrontier) > maxFrontierPerHop {
			rand.Shuffle(len(nextFrontier), func(i, j int) {
				nextFrontier[i], nextFrontier[j] = nextFrontier[j], nextFrontier[i]
			})
			nextFrontier = nextFrontier[:maxFrontierPerHop]
		}

		frontier = nextFrontier
	}

	if !found {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("committing shortest path: %w", err)
		}

		return nil, nil
	}

	// Reconstruct path from toID back to fromID using parent map.
	trail := []string{toID}
	for current := toID; current != fromID; {
		p, ok := parent[current]
		if !ok {
			break
		}

		trail = append(trail, p)
		current = p
	}

	// Reverse trail to get fromID -> toID order.
	for i, j := 0, len(trail)-1; i < j; i, j = i+1, j-1 {
		trail[i], trail[j] = trail[j], trail[i]
	}

	// Fetch all path nodes preserving trail order.
	pathSQL := `SELECT ` + nodeColumns + `
		FROM kg_nodes
		INNER JOIN unnest($1::text[]) WITH ORDINALITY AS t(id, ord) USING (id)
		WHERE kg_nodes.tenant_id = current_setting('app.tenant_id')::uuid
		ORDER BY t.ord
		LIMIT ` + fmt.Sprintf("%d", maxGraphNodeFetch)

	pathRows, err := tx.Query(ctx, pathSQL, trail)
	if err != nil {
		return nil, fmt.Errorf("querying path nodes: %w", err)
	}
	defer pathRows.Close()

	nodes, err := collectNodes(pathRows)
	if err != nil {
		return nil, fmt.Errorf("collecting path nodes: %w", err)
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing shortest path: %w", err)
	}

	return nodes, nil
}

// fetchPathNodes is a helper for the trivial case where from == to.
func (s *GraphStore) fetchPathNodes(ctx context.Context, tenantID string, ids []string) ([]models.Node, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("fetching path nodes: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	nodeSQL := `SELECT ` + nodeColumns + ` FROM kg_nodes WHERE id = ANY($1) AND tenant_id = current_setting('app.tenant_id')::uuid LIMIT ` + fmt.Sprintf("%d", maxGraphNodeFetch)

	rows, err := tx.Query(ctx, nodeSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("querying path nodes: %w", err)
	}
	defer rows.Close()

	nodes, err := collectNodes(rows)
	if err != nil {
		return nil, fmt.Errorf("collecting path nodes: %w", err)
	}

	if err := s.decryptNodes(ctx, tenantID, nodes); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing path nodes: %w", err)
	}

	return nodes, nil
}
