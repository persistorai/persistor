package models

// ReprocessNodesRequest requests batched node reprocessing for existing data.
type ReprocessNodesRequest struct {
	BatchSize  int  `json:"batch_size,omitempty"`
	SearchText bool `json:"search_text,omitempty"`
	Embeddings bool `json:"embeddings,omitempty"`
}

// ReprocessNodesResult summarizes a batched reprocessing run.
type ReprocessNodesResult struct {
	Scanned                int `json:"scanned"`
	UpdatedSearch          int `json:"updated_search"`
	QueuedEmbed            int `json:"queued_embeddings"`
	RemainingSearchText    int `json:"remaining_search_text"`
	RemainingEmbeddings    int `json:"remaining_embeddings"`
	RemainingTotal         int `json:"remaining_total"`
}
