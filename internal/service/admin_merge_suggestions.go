package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

const defaultMergeSuggestionMinScore = 0.6

// ListMergeSuggestions returns explainable duplicate candidates without performing merges.
func (s *AdminService) ListMergeSuggestions(ctx context.Context, tenantID string, opts models.MergeSuggestionListOpts) ([]models.MergeSuggestion, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}
	pairs, err := s.store.ListDuplicateCandidatePairs(ctx, tenantID, opts.Type, limit*3)
	if err != nil {
		return nil, err
	}

	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = defaultMergeSuggestionMinScore
	}

	suggestions := make([]models.MergeSuggestion, 0, min(limit, len(pairs)))
	for _, pair := range pairs {
		suggestion := buildMergeSuggestion(pair)
		if suggestion.Score < minScore {
			continue
		}
		suggestions = append(suggestions, suggestion)
	}

	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].Score == suggestions[j].Score {
			return suggestions[i].Canonical.ID < suggestions[j].Canonical.ID
		}
		return suggestions[i].Score > suggestions[j].Score
	})
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions, nil
}

func buildMergeSuggestion(pair store.DuplicateCandidatePair) models.MergeSuggestion {
	canonical, duplicate := orderSuggestionNodes(pair.Left, pair.Right)
	reasons := make([]models.MergeSuggestionReason, 0, 4)
	score := 0.0

	if pair.SameLabel {
		reasons = append(reasons, models.MergeSuggestionReason{
			Code:        "same_normalized_label",
			Description: "Nodes have the same normalized label.",
			Weight:      0.55,
			Evidence:    []string{pair.Left.Label, pair.Right.Label},
		})
		score += 0.55
	}
	if pair.LabelAliasOverlap {
		reasons = append(reasons, models.MergeSuggestionReason{
			Code:        "label_alias_overlap",
			Description: "One node's label matches the other's stored alias.",
			Weight:      0.2,
			Evidence:    trimEvidence(pair.SharedNames, 3),
		})
		score += 0.2
	}
	if len(pair.SharedNames) > 0 {
		weight := math.Min(0.3, 0.15+0.05*float64(len(pair.SharedNames)-1))
		reasons = append(reasons, models.MergeSuggestionReason{
			Code:        "shared_names",
			Description: "Nodes share normalized names or aliases.",
			Weight:      weight,
			Evidence:    trimEvidence(pair.SharedNames, 4),
		})
		score += weight
	}
	if propertyReason, ok := identityPropertyReason(pair.Left.Properties, pair.Right.Properties); ok {
		reasons = append(reasons, propertyReason)
		score += propertyReason.Weight
	}

	if score > 1 {
		score = 1
	}
	return models.MergeSuggestion{
		Canonical: summaryNode(canonical),
		Duplicate: summaryNode(duplicate),
		Score:     math.Round(score*100) / 100,
		Reasons:   reasons,
	}
}

func summaryNode(node models.Node) models.MergeSuggestionNode {
	return models.MergeSuggestionNode{ID: node.ID, Type: node.Type, Label: node.Label, Salience: node.Salience}
}

func orderSuggestionNodes(left, right models.Node) (models.Node, models.Node) {
	if right.Salience > left.Salience {
		return right, left
	}
	if right.Salience == left.Salience && right.ID < left.ID {
		return right, left
	}
	return left, right
}

func identityPropertyReason(left, right map[string]any) (models.MergeSuggestionReason, bool) {
	matches := make([]string, 0, 3)
	for key, leftValue := range left {
		if !isIdentityLikeKey(key) {
			continue
		}
		rightValue, ok := right[key]
		if !ok {
			continue
		}
		leftNorm, okLeft := normalizeIdentityValue(leftValue)
		rightNorm, okRight := normalizeIdentityValue(rightValue)
		if !okLeft || !okRight || leftNorm == "" || leftNorm != rightNorm {
			continue
		}
		matches = append(matches, fmt.Sprintf("%s=%s", key, leftNorm))
	}
	if len(matches) == 0 {
		return models.MergeSuggestionReason{}, false
	}
	weight := math.Min(0.3, 0.2+0.05*float64(len(matches)-1))
	return models.MergeSuggestionReason{
		Code:        "matching_identity_properties",
		Description: "Nodes share matching identity-like property values.",
		Weight:      weight,
		Evidence:    trimEvidence(matches, 3),
	}, true
}

func isIdentityLikeKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "email") || strings.Contains(key, "phone") || strings.Contains(key, "url") || strings.Contains(key, "domain") || strings.Contains(key, "username") || strings.Contains(key, "handle") || strings.HasSuffix(key, "_id") || key == "id"
}

func normalizeIdentityValue(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(value)), true
	case fmt.Stringer:
		return strings.ToLower(strings.TrimSpace(value.String())), true
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", value)), true
	case float32:
		return strings.TrimSpace(fmt.Sprintf("%.0f", value)), true
	case int, int8, int16, int32, int64:
		return strings.TrimSpace(fmt.Sprintf("%v", value)), true
	case uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprintf("%v", value)), true
	default:
		return "", false
	}
}

func trimEvidence(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	return values[:max]
}
