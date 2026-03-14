package ingest

import (
	"context"
	"log/slog"
	"strings"

	"github.com/persistorai/persistor/client"
)

// fuzzySearchLimit is the max results to consider from full-text search.
const fuzzySearchLimit = 10

// minFuzzyScore is the minimum similarity score to accept a fuzzy match.
const minFuzzyScore = 0.8

// companyStripSuffixes are common suffixes stripped during label normalization.
var companyStripSuffixes = []string{
	", incorporated",
	", inc.",
	", inc",
	" incorporated",
	" inc.",
	" inc",
	", llc",
	" llc",
	", ltd.",
	", ltd",
	" ltd.",
	" ltd",
	" corporation",
	", corp.",
	", corp",
	" corp.",
	" corp",
	" co.",
	" company",
	" wireless",
	" communications",
	" technologies",
	" technology",
	" solutions",
	" services",
	" group",
	" holdings",
	" enterprises",
}

// findByNameFuzzy searches for a node using full-text search and fuzzy matching.
// Returns the best matching node if the similarity score meets the threshold.
func (w *Writer) findByNameFuzzy(
	ctx context.Context,
	name string,
) (*client.Node, error) {
	candidates, err := w.graph.SearchNodes(ctx, name, fuzzySearchLimit)
	if err != nil {
		slog.Debug("fuzzy search failed, treating as no match", "name", name, "err", err)
		return nil, nil //nolint:nilerr // Search failure is non-fatal; fall through to no match
	}

	bestNode, bestScore := pickBestMatch(name, candidates)
	if bestScore < minFuzzyScore {
		return nil, nil
	}

	slog.Info("fuzzy matched entity",
		"extracted", name,
		"matched", bestNode.Label,
		"score", bestScore,
	)

	return bestNode, nil
}

// pickBestMatch finds the candidate with the highest fuzzy similarity to name.
func pickBestMatch(name string, candidates []client.Node) (best *client.Node, score float64) {
	var bestNode *client.Node
	var bestScore float64

	for i := range candidates {
		score := fuzzyMatchLabel(name, candidates[i].Label)
		if score > bestScore {
			bestScore = score
			bestNode = &candidates[i]
		}
	}

	return bestNode, bestScore
}

// fuzzyMatchLabel returns a similarity score (0.0-1.0) between two labels.
// 1.0 = exact match after normalization, 0.8 = one contains the other, 0.0 = no match.
func fuzzyMatchLabel(extracted, candidate string) float64 {
	normExtracted := normalizeLabel(extracted)
	normCandidate := normalizeLabel(candidate)

	if normExtracted == normCandidate {
		return 1.0
	}

	if strings.Contains(normExtracted, normCandidate) ||
		strings.Contains(normCandidate, normExtracted) {
		return 0.8
	}

	return 0.0
}

// normalizeLabel lowercases and strips common company suffixes from a label.
func normalizeLabel(label string) string {
	result := strings.ToLower(strings.TrimSpace(label))
	for _, suffix := range companyStripSuffixes {
		result = strings.TrimSuffix(result, suffix)
	}
	return strings.TrimSpace(result)
}
