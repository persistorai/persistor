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

// MigrateNodeRequest is the payload for migrating a node to a new ID.
type MigrateNodeRequest struct {
	NewID     string `json:"new_id"`
	NewLabel  string `json:"new_label,omitempty"`
	DeleteOld bool   `json:"delete_old"`
}

// MigrateNodeResult summarizes the outcome of a node migration.
type MigrateNodeResult struct {
	OldID         string  `json:"old_id"`
	NewID         string  `json:"new_id"`
	OutgoingEdges int     `json:"outgoing_edges"`
	IncomingEdges int     `json:"incoming_edges"`
	Salience      float64 `json:"salience"`
	OldDeleted    bool    `json:"old_deleted"`
	DryRun        bool    `json:"dry_run"`
}

// UpdateNodeRequest is the payload for updating an existing node.
type UpdateNodeRequest struct {
	Type       *string        `json:"type,omitempty"`
	Label      *string        `json:"label,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// PatchPropertiesRequest is the payload for partially updating properties.
// Keys with non-null values are added/updated; keys with null values are removed.
type PatchPropertiesRequest struct {
	Properties map[string]any `json:"properties"`
}

// Validate checks PatchPropertiesRequest fields.
func (r *PatchPropertiesRequest) Validate() error {
	if r.Properties == nil || len(r.Properties) == 0 {
		return fmt.Errorf("properties is required and must not be empty")
	}

	data, err := json.Marshal(r.Properties)
	if err != nil {
		return fmt.Errorf("invalid properties: %w", err)
	}
	if len(data) > 65536 {
		return ErrFieldTooLong("properties", 65536)
	}

	return nil
}

// MergeProperties merges patch into existing properties.
// Keys with null values are removed; all others are added/updated.
func MergeProperties(existing, patch map[string]any) map[string]any {
	if existing == nil {
		existing = make(map[string]any)
	}

	for k, v := range patch {
		if v == nil {
			delete(existing, k)
		} else {
			existing[k] = v
		}
	}

	return existing
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
