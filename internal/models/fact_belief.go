package models

const FactBeliefsProperty = "_fact_beliefs"

const (
	FactBeliefStatusSupported  = "supported"
	FactBeliefStatusContested  = "contested"
	FactBeliefStatusSuperseded = "superseded"
)

// FactBeliefState captures the current belief state for a single property.
type FactBeliefState struct {
	PreferredValue      any               `json:"preferred_value,omitempty"`
	PreferredConfidence float64           `json:"preferred_confidence"`
	EvidenceCount       int               `json:"evidence_count"`
	Status              string            `json:"status,omitempty"`
	Claims              []FactBeliefClaim `json:"claims,omitempty"`
}

// FactBeliefClaim summarizes support for one observed claim value.
type FactBeliefClaim struct {
	Value          any      `json:"value"`
	Confidence     float64  `json:"confidence"`
	EvidenceCount  int      `json:"evidence_count"`
	SourceWeight   float64  `json:"source_weight"`
	LastObservedAt string   `json:"last_observed_at,omitempty"`
	Sources        []string `json:"sources,omitempty"`
	Status         string   `json:"status,omitempty"`
	Preferred      bool     `json:"preferred,omitempty"`
}
