package client

import (
	"encoding/json"
	"time"
)

// Node represents a vertex in the knowledge graph.
type Node struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Label        string         `json:"label"`
	Properties   map[string]any `json:"properties"`
	Embedding    []float32      `json:"embedding,omitempty"`
	AccessCount  int            `json:"access_count"`
	LastAccessed *time.Time     `json:"last_accessed,omitempty"`
	Salience     float64        `json:"salience_score"`
	SupersededBy *string        `json:"superseded_by,omitempty"`
	UserBoosted  bool           `json:"user_boosted"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ScoredNode pairs a Node with a similarity score from semantic search.
type ScoredNode struct {
	Node
	Score float64 `json:"score"`
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	Relation     string         `json:"relation"`
	Properties   map[string]any `json:"properties"`
	Weight       float64        `json:"weight"`
	AccessCount  int            `json:"access_count"`
	LastAccessed *time.Time     `json:"last_accessed,omitempty"`
	Salience     float64        `json:"salience_score"`
	SupersededBy *string        `json:"superseded_by,omitempty"`
	UserBoosted  bool           `json:"user_boosted"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// CreateNodeRequest is the payload for creating a node.
type CreateNodeRequest struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties,omitempty"`
}

// UpdateNodeRequest is the payload for updating a node.
type UpdateNodeRequest struct {
	Type       *string        `json:"type,omitempty"`
	Label      *string        `json:"label,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// CreateEdgeRequest is the payload for creating an edge.
type CreateEdgeRequest struct {
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	Relation   string         `json:"relation"`
	Properties map[string]any `json:"properties,omitempty"`
	Weight     *float64       `json:"weight,omitempty"`
}

// PatchPropertiesRequest is the payload for partially updating properties.
type PatchPropertiesRequest struct {
	Properties map[string]any `json:"properties"`
}

// UpdateEdgeRequest is the payload for updating an edge.
type UpdateEdgeRequest struct {
	Properties map[string]any `json:"properties,omitempty"`
	Weight     *float64       `json:"weight,omitempty"`
}

// SupersedeRequest is the payload for superseding one node with another.
type SupersedeRequest struct {
	OldID string `json:"old_id"`
	NewID string `json:"new_id"`
}

// NeighborResult holds nodes and edges directly connected to a node.
type NeighborResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// TraverseResult holds a subgraph discovered by BFS traversal.
type TraverseResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// ContextResult holds a node with its immediate neighborhood.
type ContextResult struct {
	Node      Node   `json:"node"`
	Neighbors []Node `json:"neighbors"`
	Edges     []Edge `json:"edges"`
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Actor      string         `json:"actor,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// PropertyChange represents a single property value change.
type PropertyChange struct {
	ID          int64           `json:"id"`
	NodeID      string          `json:"node_id"`
	PropertyKey string          `json:"property_key"`
	OldValue    json.RawMessage `json:"old_value"`
	NewValue    json.RawMessage `json:"new_value"`
	ChangedAt   time.Time       `json:"changed_at"`
	Reason      *string         `json:"reason,omitempty"`
	ChangedBy   *string         `json:"changed_by,omitempty"`
}

// HealthResponse is returned by the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// StatsResponse is returned by the stats endpoint.
type StatsResponse struct {
	Nodes              int     `json:"nodes"`
	Edges              int     `json:"edges"`
	EntityTypes        int     `json:"entity_types"`
	AvgSalience        float64 `json:"avg_salience"`
	EmbeddingsComplete int     `json:"embeddings_complete"`
	EmbeddingsPending  int     `json:"embeddings_pending"`
}

// ListOptions holds common pagination parameters.
type ListOptions struct {
	Limit  int
	Offset int
}

// NodeListOptions holds parameters for listing nodes.
type NodeListOptions struct {
	Type        string
	MinSalience float64
	Limit       int
	Offset      int
}

// EdgeListOptions holds parameters for listing edges.
type EdgeListOptions struct {
	Source   string
	Target   string
	Relation string
	Limit    int
	Offset   int
}

// SearchOptions holds parameters for search queries.
type SearchOptions struct {
	Type        string
	MinSalience float64
	Limit       int
}

// AuditQueryOptions holds parameters for querying audit logs.
type AuditQueryOptions struct {
	EntityType string
	EntityID   string
	Action     string
	Since      *time.Time
	Limit      int
	Offset     int
}
