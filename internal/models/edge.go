package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Edge represents a directed relationship between two nodes.
type Edge struct {
	TenantID     uuid.UUID      `json:"-"`
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

// CreateEdgeRequest is the payload for creating a new edge.
type CreateEdgeRequest struct {
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	Relation   string         `json:"relation"`
	Properties map[string]any `json:"properties,omitempty"`
	Weight     *float64       `json:"weight,omitempty"`
}

// Validate checks that required fields are present and within limits on CreateEdgeRequest.
func (r *CreateEdgeRequest) Validate() error {
	if r.Source == "" {
		return ErrMissingSource
	}

	if len(r.Source) > 255 {
		return ErrFieldTooLong("source", 255)
	}

	if r.Target == "" {
		return ErrMissingTarget
	}

	if len(r.Target) > 255 {
		return ErrFieldTooLong("target", 255)
	}

	if r.Relation == "" {
		return ErrMissingRelation
	}

	if len(r.Relation) > 255 {
		return ErrFieldTooLong("relation", 255)
	}

	if r.Weight != nil && (*r.Weight < 0 || *r.Weight > 1000) {
		return fmt.Errorf("weight must be between 0 and 1000")
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

// UpdateEdgeRequest is the payload for updating an existing edge.
type UpdateEdgeRequest struct {
	Properties map[string]any `json:"properties,omitempty"`
	Weight     *float64       `json:"weight,omitempty"`
}

// Validate checks UpdateEdgeRequest fields.
func (r *UpdateEdgeRequest) Validate() error {
	if r.Weight != nil && (*r.Weight < 0 || *r.Weight > 1000) {
		return fmt.Errorf("weight must be between 0 and 1000")
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
