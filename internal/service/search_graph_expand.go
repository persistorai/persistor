package service

import (
	"context"
	"sort"

	"github.com/persistorai/persistor/internal/models"
)

const defaultGraphExpansionLimit = 3

// GraphLookupStore is the narrow graph capability SearchService can optionally use
// for neighborhood-aware retrieval expansion.
type GraphLookupStore interface {
	Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
}

func mergeExpandedNodes(primary []models.Node, expanded []models.Node, limit int) []models.Node {
	if limit <= 0 {
		limit = len(primary) + len(expanded)
	}

	seen := make(map[string]struct{}, len(primary)+len(expanded))
	merged := make([]models.Node, 0, min(limit, len(primary)+len(expanded)))

	appendUnique := func(nodes []models.Node) {
		for _, node := range nodes {
			if _, ok := seen[node.ID]; ok {
				continue
			}
			seen[node.ID] = struct{}{}
			merged = append(merged, node)
			if len(merged) >= limit {
				return
			}
		}
	}

	appendUnique(primary)
	if len(merged) >= limit {
		return merged
	}

	sort.SliceStable(expanded, func(i, j int) bool {
		if expanded[i].Salience == expanded[j].Salience {
			return expanded[i].UpdatedAt.After(expanded[j].UpdatedAt)
		}
		return expanded[i].Salience > expanded[j].Salience
	})
	appendUnique(expanded)

	return merged
}

func (s *SearchService) expandFromGraph(ctx context.Context, tenantID string, seeds []models.Node, limit int) []models.Node {
	if s.graph == nil || len(seeds) == 0 || limit <= 0 {
		return nil
	}

	expanded := make([]models.Node, 0, limit*defaultGraphExpansionLimit)
	for _, seed := range seeds {
		neighbors, err := s.graph.Neighbors(ctx, tenantID, seed.ID, defaultGraphExpansionLimit)
		if err != nil || neighbors == nil || len(neighbors.Nodes) == 0 {
			continue
		}
		expanded = append(expanded, neighbors.Nodes...)
	}

	return expanded
}
