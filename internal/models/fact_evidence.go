package models

const (
	FactUpdatesProperty  = "_fact_updates"
	FactEvidenceProperty = "_fact_evidence"
)

// FactUpdate carries evidence metadata for an incoming fact without changing public endpoints.
type FactUpdate struct {
	Value      any      `json:"value"`
	Source     string   `json:"source,omitempty"`
	Timestamp  string   `json:"timestamp,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// FactEvidence records an observed fact and whether it conflicted with or superseded a prior value.
type FactEvidence struct {
	Value                      any      `json:"value"`
	Source                     string   `json:"source,omitempty"`
	Timestamp                  string   `json:"timestamp,omitempty"`
	Confidence                 *float64 `json:"confidence,omitempty"`
	ConflictsWithPrior         bool     `json:"conflicts_with_prior,omitempty"`
	SupersedesPrior            bool     `json:"supersedes_prior,omitempty"`
	PreviousValue              any      `json:"previous_value,omitempty"`
	HistoricalEvidenceRetained bool     `json:"historical_evidence_retained,omitempty"`
}
