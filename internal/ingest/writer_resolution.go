package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/persistorai/persistor/client"
)

const (
	resolverSearchLimit        = 10
	resolverAutoMatchThreshold = 0.93
	resolverCandidateThreshold = 0.5
	resolverAmbiguousGap       = 0.08
)

type entityResolutionStatus string

type matchMethod string

const (
	resolutionMatched   entityResolutionStatus = "matched"
	resolutionAmbiguous entityResolutionStatus = "ambiguous"
	resolutionNoMatch   entityResolutionStatus = "no_match"

	matchMethodExactID              matchMethod = "exact_id"
	matchMethodExactLabel           matchMethod = "exact_label"
	matchMethodExactNormalizedLabel matchMethod = "exact_normalized_label"
	matchMethodAlias                matchMethod = "alias"
	matchMethodFuzzy                matchMethod = "fuzzy"
)

type entityMatchCandidate struct {
	Node       *client.Node
	Method     matchMethod
	Confidence float64
	Signals    []string
}

type entityResolution struct {
	Query           string
	NormalizedQuery string
	ExpectedType    string
	Status          entityResolutionStatus
	Match           *entityMatchCandidate
	Candidates      []entityMatchCandidate
}

func (w *Writer) resolveEntity(ctx context.Context, query, expectedType string) (*entityResolution, error) {
	trimmed := strings.TrimSpace(query)
	resolution := &entityResolution{
		Query:           trimmed,
		NormalizedQuery: normalizeLabel(trimmed),
		ExpectedType:    strings.TrimSpace(expectedType),
		Status:          resolutionNoMatch,
	}
	if trimmed == "" {
		return resolution, nil
	}

	if direct, err := w.graph.GetNode(ctx, trimmed); err == nil && direct != nil {
		candidate := scoreResolutionCandidate(trimmed, resolution.NormalizedQuery, resolution.ExpectedType, direct, direct.ID == trimmed)
		candidate.Method = matchMethodExactID
		candidate.Confidence = 1.0
		candidate.Signals = append([]string{"exact id match"}, candidate.Signals...)
		resolution.Status = resolutionMatched
		resolution.Match = &candidate
		resolution.Candidates = []entityMatchCandidate{candidate}
		return resolution, nil
	}

	if exact, err := w.graph.GetNodeByLabel(ctx, trimmed); err == nil && exact != nil {
		candidate := scoreResolutionCandidate(trimmed, resolution.NormalizedQuery, resolution.ExpectedType, exact, false)
		if !strings.EqualFold(strings.TrimSpace(trimmed), strings.TrimSpace(exact.Label)) {
			candidate.Method = matchMethodAlias
			candidate.Confidence = clamp01(maxFloat(candidate.Confidence, 0.96))
			candidate.Signals = append([]string{"alias-aware exact lookup"}, candidate.Signals...)
		}
		resolution.Status = resolutionMatched
		resolution.Match = &candidate
		resolution.Candidates = []entityMatchCandidate{candidate}
		return resolution, nil
	}

	candidates, err := w.graph.SearchNodes(ctx, trimmed, resolverSearchLimit)
	if err != nil {
		slog.Debug("entity candidate search failed", "query", trimmed, "err", err)
		return resolution, nil
	}

	scored := make([]entityMatchCandidate, 0, len(candidates))
	for i := range candidates {
		candidate := scoreResolutionCandidate(trimmed, resolution.NormalizedQuery, resolution.ExpectedType, &candidates[i], false)
		if candidate.Confidence < resolverCandidateThreshold {
			continue
		}
		scored = append(scored, candidate)
	}
	if len(scored) == 0 {
		return resolution, nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Confidence == scored[j].Confidence {
			if scored[i].Node.Salience == scored[j].Node.Salience {
				return scored[i].Node.UpdatedAt.After(scored[j].Node.UpdatedAt)
			}
			return scored[i].Node.Salience > scored[j].Node.Salience
		}
		return scored[i].Confidence > scored[j].Confidence
	})
	resolution.Candidates = scored
	best := scored[0]
	resolution.Match = &best
	if len(scored) > 1 && best.Confidence >= resolverCandidateThreshold && best.Confidence < resolverAutoMatchThreshold {
		if best.Confidence-scored[1].Confidence < resolverAmbiguousGap {
			resolution.Status = resolutionAmbiguous
			return resolution, nil
		}
	}
	if len(scored) > 1 && best.Confidence >= resolverAutoMatchThreshold && best.Confidence-scored[1].Confidence < resolverAmbiguousGap/2 {
		resolution.Status = resolutionAmbiguous
		return resolution, nil
	}
	if best.Confidence >= resolverAutoMatchThreshold {
		resolution.Status = resolutionMatched
		return resolution, nil
	}
	if len(scored) > 1 {
		resolution.Status = resolutionAmbiguous
		return resolution, nil
	}
	return resolution, nil
}

func scoreResolutionCandidate(query, normalizedQuery, expectedType string, node *client.Node, exactID bool) entityMatchCandidate {
	normalizedLabel := normalizeLabel(node.Label)
	confidence := trigramSimilarity(normalizedQuery, normalizedLabel) * 0.84
	method := matchMethodFuzzy
	signals := make([]string, 0, 4)

	switch {
	case exactID:
		confidence = 1.0
		method = matchMethodExactID
		signals = append(signals, "exact id match")
	case strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(node.Label)):
		confidence = 0.99
		method = matchMethodExactLabel
		signals = append(signals, "exact label match")
	case normalizedQuery != "" && normalizedQuery == normalizedLabel:
		confidence = 0.97
		method = matchMethodExactNormalizedLabel
		signals = append(signals, "normalized label match")
	case strings.Contains(normalizedQuery, normalizedLabel) || strings.Contains(normalizedLabel, normalizedQuery):
		confidence = 0.88
		method = matchMethodFuzzy
		signals = append(signals, "contains match")
	default:
		signals = append(signals, fmt.Sprintf("trigram similarity %.2f", confidence))
	}

	if expectedType != "" {
		switch {
		case strings.EqualFold(node.Type, expectedType):
			confidence += 0.06
			signals = append(signals, "type match")
		case node.Type != "":
			confidence -= 0.08
			signals = append(signals, "type mismatch")
		}
	}

	confidence = clamp01(confidence)
	if method == matchMethodExactNormalizedLabel && !strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(node.Label)) {
		method = matchMethodAlias
		signals = append([]string{"alias-aware lookup candidate"}, signals...)
	}

	return entityMatchCandidate{
		Node:       node,
		Method:     method,
		Confidence: confidence,
		Signals:    signals,
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func trigramSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	aCounts := buildTrigramCounts(a)
	bCounts := buildTrigramCounts(b)
	if len(aCounts) == 0 || len(bCounts) == 0 {
		if a == b {
			return 1
		}
		return 0
	}
	shared := 0
	for gram, aCount := range aCounts {
		if bCount, ok := bCounts[gram]; ok {
			if aCount < bCount {
				shared += aCount
			} else {
				shared += bCount
			}
		}
	}
	total := 0
	for _, count := range aCounts {
		total += count
	}
	for _, count := range bCounts {
		total += count
	}
	if total == 0 {
		return 0
	}
	return float64(shared*2) / float64(total)
}

func buildTrigramCounts(s string) map[string]int {
	runes := []rune("  " + s + "  ")
	if len(runes) < 3 {
		return map[string]int{}
	}
	grams := make(map[string]int, len(runes)-2)
	for i := 0; i+2 < len(runes); i++ {
		grams[string(runes[i:i+3])]++
	}
	return grams
}
