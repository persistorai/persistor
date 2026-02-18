package models

import "time"

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID         int64          `json:"id"`
	TenantID   string         `json:"-"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Actor      string         `json:"actor,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// AuditQueryOpts holds filters for querying the audit log.
type AuditQueryOpts struct {
	EntityType string
	EntityID   string
	Action     string
	Since      *time.Time
	Limit      int
	Offset     int
}
