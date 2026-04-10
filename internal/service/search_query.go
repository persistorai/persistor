package service

import (
	"regexp"
	"sort"
	"strings"
)

var yearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// BuildSearchQueryVariants returns deterministic query variants ordered from most
// specific to broadest. Variants are used to improve retrieval without changing
// the public API contract.
func BuildSearchQueryVariants(query string) []string {
	base := normalizeSearchQuery(query)
	if base == "" {
		return nil
	}

	variants := []string{base}
	intent := DetectSearchIntent(base)
	tokens := strings.Fields(base)

	if len(tokens) > 2 {
		variants = append(variants, strings.Join(tokens[:min(6, len(tokens))], " "))
	}

	switch intent {
	case SearchIntentEntity:
		variants = append(variants, stripLeadPhrase(base, "who is "))
		variants = append(variants, stripLeadPhrase(base, "what is "))
		variants = append(variants, stripLeadPhrase(base, "tell me about "))
	case SearchIntentProcedural:
		variants = append(variants, proceduralCore(base))
	case SearchIntentTemporal:
		variants = append(variants, temporalCore(base))
	case SearchIntentRelationship:
		variants = append(variants, relationshipCore(base))
	}

	for _, year := range yearPattern.FindAllString(base, -1) {
		variants = append(variants, year)
	}

	if len(tokens) > 0 {
		for _, token := range tokens {
			if len(token) >= 5 && !isStopwordToken(token) {
				variants = append(variants, token)
			}
		}
	}

	return dedupeQueries(variants)
}

func normalizeSearchQuery(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	query = strings.NewReplacer("?", "", ".", "", ",", "", ":", " ").Replace(query)
	return strings.Join(strings.Fields(query), " ")
}

func stripLeadPhrase(query, prefix string) string {
	if strings.HasPrefix(query, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(query, prefix))
	}
	return ""
}

func proceduralCore(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimPrefix(query, "what is ")
	query = strings.TrimPrefix(query, "how ")
	query = strings.TrimPrefix(query, "how to ")
	replacements := []string{"stance on", "preference for", "policy on", "rule about", "approach to"}
	for _, r := range replacements {
		query = strings.ReplaceAll(query, r, "")
	}
	return strings.TrimSpace(query)
}

func temporalCore(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimPrefix(query, "what happened on ")
	query = strings.TrimPrefix(query, "what happened ")
	query = strings.TrimPrefix(query, "when did ")
	query = strings.TrimPrefix(query, "history of ")
	return strings.TrimSpace(query)
}

func relationshipCore(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimPrefix(query, "what is the relationship between ")
	query = strings.TrimPrefix(query, "relationship between ")
	query = strings.ReplaceAll(query, " and ", " ")
	return strings.TrimSpace(query)
}

func dedupeQueries(variants []string) []string {
	seen := make(map[string]struct{}, len(variants))
	out := make([]string, 0, len(variants))
	for _, variant := range variants {
		variant = normalizeSearchQuery(variant)
		if variant == "" {
			continue
		}
		if _, ok := seen[variant]; ok {
			continue
		}
		seen[variant] = struct{}{}
		out = append(out, variant)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i]) > len(out[j])
	})
	return out
}

func isStopwordToken(token string) bool {
	switch token {
	case "about", "between", "brians", "brian's", "current", "during", "happened", "policy", "stance", "their", "there", "these", "those", "where", "which":
		return true
	default:
		return false
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
