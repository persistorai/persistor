package models

// ReprocessNodesRequest requests batched node reprocessing for existing data.
type ReprocessNodesRequest struct {
	BatchSize  int  `json:"batch_size,omitempty"`
	SearchText bool `json:"search_text,omitempty"`
	Embeddings bool `json:"embeddings,omitempty"`
}

// ReprocessNodesResult summarizes a batched reprocessing run.
type ReprocessNodesResult struct {
	Scanned             int `json:"scanned"`
	UpdatedSearch       int `json:"updated_search"`
	QueuedEmbed         int `json:"queued_embeddings"`
	RemainingSearchText int `json:"remaining_search_text"`
	RemainingEmbeddings int `json:"remaining_embeddings"`
	RemainingTotal      int `json:"remaining_total"`
}

// MaintenanceRunRequest triggers an explicit admin maintenance pass over existing nodes.
type MaintenanceRunRequest struct {
	BatchSize                  int  `json:"batch_size,omitempty"`
	RefreshSearchText          bool `json:"refresh_search_text,omitempty"`
	RefreshEmbeddings          bool `json:"refresh_embeddings,omitempty"`
	ScanStaleFacts             bool `json:"scan_stale_facts,omitempty"`
	IncludeDuplicateCandidates bool `json:"include_duplicate_candidates,omitempty"`
}

// MaintenanceRunResult summarizes an explicit maintenance pass.
type MaintenanceRunResult struct {
	Scanned                   int `json:"scanned"`
	UpdatedSearchText         int `json:"updated_search_text"`
	QueuedEmbeddings          int `json:"queued_embeddings"`
	StaleFactNodes            int `json:"stale_fact_nodes"`
	SupersededNodes           int `json:"superseded_nodes"`
	DuplicateCandidatePairs   int `json:"duplicate_candidate_pairs,omitempty"`
	RemainingSearchText       int `json:"remaining_search_text"`
	RemainingEmbeddings       int `json:"remaining_embeddings"`
	RemainingMaintenanceNodes int `json:"remaining_maintenance_nodes"`
}
