package models

import (
	"time"

	"github.com/google/uuid"
)

// UnknownRelation tracks LLM-generated relation types not in the canonical list.
type UnknownRelation struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"-"`
	RelationType string    `json:"relation_type"`
	SourceName   string    `json:"source_name"`
	TargetName   string    `json:"target_name"`
	SourceText   string    `json:"source_text,omitempty"`
	Count        int       `json:"count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Resolved     bool      `json:"resolved"`
	ResolvedAs   *string   `json:"resolved_as,omitempty"`
}

// UnknownRelationListOpts controls filtering and pagination for listing unknown relations.
type UnknownRelationListOpts struct {
	ResolvedOnly bool
	Limit        int
	Offset       int
}
