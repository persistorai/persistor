// Package models defines data types for the knowledge graph.
package models

import "time"

// ExportFormat is the top-level structure for a Persistor export file.
// This is a full-fidelity export — all data including embeddings and usage
// metrics are preserved. Import flags control what gets reset.
type ExportFormat struct {
	SchemaVersion    int          `json:"schema_version"`    // Current schema migration version
	PersistorVersion string       `json:"persistor_version"` // Persistor binary version
	ExportedAt       time.Time    `json:"exported_at"`
	TenantID         string       `json:"tenant_id"`
	Stats            ExportStats  `json:"stats"`
	Nodes            []ExportNode `json:"nodes"`
	Edges            []ExportEdge `json:"edges"`
}

// ExportStats summarises the contents of an export.
type ExportStats struct {
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
}

// ExportNode is the portable representation of a node in an export file.
// Includes all fields for full-fidelity backup/restore.
type ExportNode struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Label         string         `json:"label"`
	Properties    map[string]any `json:"properties"`
	Embedding     []float32      `json:"embedding,omitempty"`     // pgvector embedding (1024-dim)
	AccessCount   int            `json:"access_count"`
	LastAccessed  *time.Time     `json:"last_accessed,omitempty"`
	SalienceScore float64        `json:"salience_score"`
	UserBoosted   bool           `json:"user_boosted"`
	SupersededBy  *string        `json:"superseded_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// ExportEdge is the portable representation of an edge in an export file.
type ExportEdge struct {
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	Relation     string         `json:"relation"`
	Properties   map[string]any `json:"properties"`
	Weight       float64        `json:"weight"`
	AccessCount  int            `json:"access_count"`
	LastAccessed *time.Time     `json:"last_accessed,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ImportResult summarises the outcome of an import operation.
type ImportResult struct {
	NodesCreated int      `json:"nodes_created"`
	NodesUpdated int      `json:"nodes_updated"`
	NodesSkipped int      `json:"nodes_skipped"`
	EdgesCreated int      `json:"edges_created"`
	EdgesUpdated int      `json:"edges_updated"`
	EdgesSkipped int      `json:"edges_skipped"`
	Errors       []string `json:"errors,omitempty"`
}

// ImportOptions controls the behaviour of an import operation.
type ImportOptions struct {
	// OverwriteExisting updates nodes/edges that already exist; otherwise they are skipped.
	OverwriteExisting bool `json:"overwrite_existing"`
	// DryRun validates the import data without writing anything to the database.
	DryRun bool `json:"dry_run"`
	// RegenerateEmbeddings re-generates embeddings even if the export includes them.
	// Use when the embedding model has changed since the export was created.
	RegenerateEmbeddings bool `json:"regenerate_embeddings"`
	// ResetUsage zeroes out access_count and last_accessed on imported nodes/edges.
	// Use when importing into a fresh instance where usage metrics should start clean.
	ResetUsage bool `json:"reset_usage"`
}
