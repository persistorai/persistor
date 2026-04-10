package models

// MergeSuggestionListOpts controls duplicate-candidate and merge-suggestion listing.
type MergeSuggestionListOpts struct {
	Type     string  `json:"type,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	MinScore float64 `json:"min_score,omitempty"`
}

// MergeSuggestionNode is the compact node representation returned in merge suggestions.
type MergeSuggestionNode struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Label    string  `json:"label"`
	Salience float64 `json:"salience_score"`
}

// MergeSuggestionReason explains why a pair was flagged as a likely duplicate.
type MergeSuggestionReason struct {
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Weight      float64  `json:"weight"`
	Evidence    []string `json:"evidence,omitempty"`
}

// MergeSuggestion represents an explainable duplicate candidate pair.
type MergeSuggestion struct {
	Canonical MergeSuggestionNode     `json:"canonical"`
	Duplicate MergeSuggestionNode     `json:"duplicate"`
	Score     float64                 `json:"score"`
	Reasons   []MergeSuggestionReason `json:"reasons"`
}
