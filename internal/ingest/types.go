package ingest

// ExtractionResult holds the structured output from LLM extraction.
type ExtractionResult struct {
	Entities      []ExtractedEntity       `json:"entities"`
	Relationships []ExtractedRelationship `json:"relationships"`
	Facts         []ExtractedFact         `json:"facts"`
}

// ExtractedEntity is a named entity found in text.
type ExtractedEntity struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Properties  map[string]any `json:"properties"`
	Description string         `json:"description"`
}

// ExtractedRelationship is a directed relation between two entities.
type ExtractedRelationship struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Relation   string  `json:"relation"`
	Confidence float64 `json:"confidence"`
	DateStart  *string `json:"date_start,omitempty"`
	DateEnd    *string `json:"date_end,omitempty"`
	IsCurrent  *bool   `json:"is_current,omitempty"`
}

// ExtractedFact is a key-value fact about an entity.
// Value is typed as any to support string, bool, and numeric values from LLM extraction.
type ExtractedFact struct {
	Subject  string `json:"subject"`
	Property string `json:"property"`
	Value    any    `json:"value"`
}
