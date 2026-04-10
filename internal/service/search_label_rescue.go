package service

import (
	"context"
	"strings"

	"github.com/persistorai/persistor/internal/models"
)

// LabelLookupStore is the narrow label lookup capability SearchService can optionally use
// for retrieval rescue on abstract or policy-style prompts.
type LabelLookupStore interface {
	GetNodeByLabel(ctx context.Context, tenantID, label string) (*models.Node, error)
}

func candidateLabelsFromQuery(query string) []string {
	variants := BuildSearchQueryVariants(query)
	labels := make([]string, 0, len(variants)+2)
	for _, variant := range variants {
		label := humanizeVariant(variant)
		if label == "" {
			continue
		}
		labels = append(labels, label)
	}
	labels = append(labels, extractKnownPhrase(query, "Dirt Road Systems"))
	labels = append(labels, extractKnownPhrase(query, "Brian"))
	labels = append(labels, extractKnownPhrase(query, "Big Jerry"))
	labels = append(labels, extractKnownPhrase(query, "Persistor"))
	labels = append(labels, extractKnownPhrase(query, "DeerPrint"))
	return dedupeLabels(labels)
}

func humanizeVariant(variant string) string {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return ""
	}
	parts := strings.Fields(variant)
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func extractKnownPhrase(query, phrase string) string {
	if strings.Contains(strings.ToLower(query), strings.ToLower(phrase)) {
		return phrase
	}
	return ""
}

func dedupeLabels(labels []string) []string {
	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		key := strings.ToLower(strings.TrimSpace(label))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
	}
	return out
}

func (s *SearchService) rescueByLabel(ctx context.Context, tenantID, query string) []models.Node {
	lookup, ok := s.store.(LabelLookupStore)
	if !ok {
		return nil
	}

	found := make([]models.Node, 0, 3)
	for _, candidate := range candidateLabelsFromQuery(query) {
		node, err := lookup.GetNodeByLabel(ctx, tenantID, candidate)
		if err != nil || node == nil {
			continue
		}
		found = append(found, *node)
		if len(found) >= 3 {
			break
		}
	}
	return found
}
