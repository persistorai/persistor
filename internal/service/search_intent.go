package service

import "strings"

// SearchIntent classifies a retrieval query into a coarse intent bucket.
type SearchIntent string

const (
	SearchIntentGeneral      SearchIntent = "general"
	SearchIntentEntity       SearchIntent = "entity"
	SearchIntentTemporal     SearchIntent = "temporal"
	SearchIntentRelationship SearchIntent = "relationship"
	SearchIntentProcedural   SearchIntent = "procedural"
)

// DetectSearchIntent returns a lightweight deterministic intent classification.
func DetectSearchIntent(query string) SearchIntent {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return SearchIntentGeneral
	}

	temporalHints := []string{"when ", "what happened", "history", "timeline", "before ", "after ", "during ", "current", "currently"}
	for _, hint := range temporalHints {
		if strings.Contains(q, hint) {
			return SearchIntentTemporal
		}
	}

	relationshipHints := []string{"related", "relationship", "connected", "path", "between", "with", "who works with", "who knows"}
	for _, hint := range relationshipHints {
		if strings.Contains(q, hint) {
			return SearchIntentRelationship
		}
	}

	proceduralHints := []string{"how ", "how to", "policy", "rule", "preference", "stance", "approach"}
	for _, hint := range proceduralHints {
		if strings.Contains(q, hint) {
			return SearchIntentProcedural
		}
	}

	entityHints := []string{"who is", "what is", "tell me about"}
	for _, hint := range entityHints {
		if strings.Contains(q, hint) {
			return SearchIntentEntity
		}
	}

	return SearchIntentGeneral
}
