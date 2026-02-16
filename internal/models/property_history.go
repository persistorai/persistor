package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PropertyChange represents a single property value change on a node.
type PropertyChange struct {
	ID          int64           `json:"id"`
	TenantID    uuid.UUID       `json:"-"`
	NodeID      string          `json:"node_id"`
	PropertyKey string          `json:"property_key"`
	OldValue    json.RawMessage `json:"old_value"`
	NewValue    json.RawMessage `json:"new_value"`
	ChangedAt   time.Time       `json:"changed_at"`
	Reason      *string         `json:"reason,omitempty"`
	ChangedBy   *string         `json:"changed_by,omitempty"`
}

// PropertyHistoryQuery holds query parameters for property history lookups.
type PropertyHistoryQuery struct {
	NodeID      string
	PropertyKey string // optional filter
	Limit       int
	Offset      int
}
