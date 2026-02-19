package graphql

import (
	"strconv"

	"github.com/persistorai/persistor/internal/models"
)

// nodeToGQL converts a models.Node to the GraphQL Node type.
func nodeToGQL(n *models.Node) *Node {
	if n == nil {
		return nil
	}
	return &Node{
		ID:            n.ID,
		Type:          n.Type,
		Label:         n.Label,
		Properties:    n.Properties,
		AccessCount:   n.AccessCount,
		SalienceScore: n.Salience,
		UserBoosted:   n.UserBoosted,
		CreatedAt:     n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// nodesToGQL converts a slice of models.Node to GraphQL Node pointers.
func nodesToGQL(nodes []models.Node) []*Node {
	out := make([]*Node, len(nodes))
	for i := range nodes {
		out[i] = nodeToGQL(&nodes[i])
	}
	return out
}

// edgeToGQL converts a models.Edge to the GraphQL Edge type.
func edgeToGQL(e *models.Edge) *Edge {
	if e == nil {
		return nil
	}
	return &Edge{
		Source:        e.Source,
		Target:        e.Target,
		Relation:      e.Relation,
		Properties:    e.Properties,
		Weight:        e.Weight,
		AccessCount:   e.AccessCount,
		SalienceScore: e.Salience,
		UserBoosted:   e.UserBoosted,
		CreatedAt:     e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// edgesToGQL converts a slice of models.Edge to GraphQL Edge pointers.
func edgesToGQL(edges []models.Edge) []*Edge {
	out := make([]*Edge, len(edges))
	for i := range edges {
		out[i] = edgeToGQL(&edges[i])
	}
	return out
}

// auditToGQL converts a models.AuditEntry to the GraphQL AuditEntry type.
func auditToGQL(a *models.AuditEntry) *AuditEntry {
	if a == nil {
		return nil
	}
	entry := &AuditEntry{
		ID:         strconv.FormatInt(a.ID, 10),
		Action:     a.Action,
		EntityType: a.EntityType,
		EntityID:   a.EntityID,
		Detail:     a.Detail,
		CreatedAt:  a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if a.Actor != "" {
		entry.Actor = &a.Actor
	}
	return entry
}

// nodesToSearchResults wraps nodes as SearchResults with zero scores
// (used for full-text and hybrid search where per-result scores are not
// yet returned from the store layer).
func nodesToSearchResults(nodes []models.Node) []*SearchResult {
	out := make([]*SearchResult, len(nodes))
	for i := range nodes {
		out[i] = &SearchResult{Node: nodeToGQL(&nodes[i]), Score: 0}
	}
	return out
}

// scoredNodesToSearchResults converts ScoredNodes (from semantic search) to
// GraphQL SearchResults with real relevance scores.
func scoredNodesToSearchResults(scored []models.ScoredNode) []*SearchResult {
	out := make([]*SearchResult, len(scored))
	for i := range scored {
		out[i] = &SearchResult{Node: nodeToGQL(&scored[i].Node), Score: scored[i].Score}
	}
	return out
}

// deref returns the value pointed to by p, or fallback if p is nil.
func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// derefStr returns the string pointed to by p, or empty string if nil.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
