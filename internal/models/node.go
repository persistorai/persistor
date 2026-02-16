// Package models defines data types for the knowledge graph.
package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Node represents a vertex in the knowledge graph.
type Node struct {
	ID           string         `json:"id"`
	TenantID     uuid.UUID      `json:"-"`
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

// NodeSummary is a lightweight representation for batch operations (backfill, etc.).
type NodeSummary struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

// EmbeddingText returns the text to embed for this node: "type:label".
func (n *NodeSummary) EmbeddingText() string {
	return n.Type + ":" + n.Label
}

// ScoredNode pairs a Node with a similarity score from semantic search.
type ScoredNode struct {
	Node
	Score float64 `json:"score"`
}

// CreateNodeRequest is the payload for creating a new node.
type CreateNodeRequest struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Validate checks that required fields are present and within limits on CreateNodeRequest.
// If ID is empty, a UUID is auto-generated.
func (r *CreateNodeRequest) Validate() error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}

	if len(r.ID) > 255 {
		return ErrFieldTooLong("id", 255)
	}

	if r.Type == "" {
		return ErrMissingType
	}

	if len(r.Type) > 100 {
		return ErrFieldTooLong("type", 100)
	}

	if r.Label == "" {
		return ErrMissingLabel
	}

	if len(r.Label) > 10000 {
		return ErrFieldTooLong("label", 10000)
	}

	if r.Properties != nil {
		data, err := json.Marshal(r.Properties)
		if err != nil {
			return fmt.Errorf("invalid properties: %w", err)
		}
		if len(data) > 65536 {
			return ErrFieldTooLong("properties", 65536)
		}
	}

	return nil
}

// UpdateNodeRequest is the payload for updating an existing node.
type UpdateNodeRequest struct {
	Type       *string        `json:"type,omitempty"`
	Label      *string        `json:"label,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Validate checks UpdateNodeRequest fields.
func (r *UpdateNodeRequest) Validate() error {
	if r.Type != nil && *r.Type == "" {
		return fmt.Errorf("type cannot be empty")
	}

	if r.Label != nil && *r.Label == "" {
		return fmt.Errorf("label cannot be empty")
	}

	if r.Type != nil && len(*r.Type) > 100 {
		return ErrFieldTooLong("type", 100)
	}

	if r.Label != nil && len(*r.Label) > 10000 {
		return ErrFieldTooLong("label", 10000)
	}

	if r.Properties != nil {
		data, err := json.Marshal(r.Properties)
		if err != nil {
			return fmt.Errorf("invalid properties: %w", err)
		}
		if len(data) > 65536 {
			return ErrFieldTooLong("properties", 65536)
		}
	}

	return nil
}
