package ingest

import "strings"

var relationAliases = map[string]string{
	"helps_build": "works_on",
	"building":    "works_on",
	"builds":      "works_on",
	"built":       "created",
	"creator_of":  "created",
	"belongs_to":  "product_of",
	"owned_by":    "product_of",
	"employed_at": "works_at",
	"lives_in":    "located_in",
	"resides_in":  "located_in",
}

func normalizeRelation(rel string) string {
	trimmed := strings.TrimSpace(strings.ToLower(rel))
	if trimmed == "" {
		return ""
	}
	if canonical, ok := relationAliases[trimmed]; ok {
		return canonical
	}
	return trimmed
}
