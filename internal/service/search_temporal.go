package service

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

const temporalScoreCap = 0.9

type temporalQueryProfile struct {
	PreferRecent     bool
	PreferHistorical bool
	YearHints        []int
}

type temporalRange struct {
	freshest time.Time
	oldest   time.Time
	hasRange bool
}

func shapeTemporalNodes(query string, nodes []models.Node, limit int) []models.Node {
	if len(nodes) == 0 || limit <= 0 {
		return nodes
	}

	profile := buildTemporalQueryProfile(query)
	if !profile.PreferRecent && !profile.PreferHistorical && len(profile.YearHints) == 0 {
		return shapeBeliefAwareNodes(nodes, limit)
	}

	freshestBelief := freshestBeliefObservation(nodes)
	rangeInfo := nodeTemporalRange(nodes)
	ranked := make([]rerankedNode, 0, len(nodes))
	for idx, node := range nodes {
		ranked = append(ranked, rerankedNode{
			node: node,
			score: beliefAwareNodeScore(node, idx, freshestBelief, defaultBeliefAwareWeights) +
				temporalNodeScore(node, idx, profile, rangeInfo),
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if math.Abs(ranked[i].score-ranked[j].score) > 1e-9 {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].node.Salience != ranked[j].node.Salience {
			return ranked[i].node.Salience > ranked[j].node.Salience
		}
		if !ranked[i].node.UpdatedAt.Equal(ranked[j].node.UpdatedAt) {
			return ranked[i].node.UpdatedAt.After(ranked[j].node.UpdatedAt)
		}
		return ranked[i].node.ID < ranked[j].node.ID
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]models.Node, 0, len(ranked))
	for _, item := range ranked {
		results = append(results, item.node)
	}
	return results
}

func buildTemporalQueryProfile(query string) temporalQueryProfile {
	normalized := normalizeSearchQuery(query)
	profile := temporalQueryProfile{}
	for _, year := range yearPattern.FindAllString(normalized, -1) {
		if parsed, err := time.Parse("2006", year); err == nil {
			profile.YearHints = append(profile.YearHints, parsed.Year())
		}
	}

	recentHints := []string{"recent", "recently", "latest", "current", "currently", "today", "now", "newest"}
	for _, hint := range recentHints {
		if strings.Contains(normalized, hint) {
			profile.PreferRecent = true
			break
		}
	}

	historicalHints := []string{"history", "historical", "timeline", "archive", "archived", "earliest", "first", "original", "initial", "previously"}
	for _, hint := range historicalHints {
		if strings.Contains(normalized, hint) {
			profile.PreferHistorical = true
			break
		}
	}

	if len(profile.YearHints) > 0 && !profile.PreferRecent {
		profile.PreferHistorical = true
	}

	return profile
}

func nodeTemporalRange(nodes []models.Node) temporalRange {
	var out temporalRange
	for i, node := range nodes {
		anchor := temporalAnchor(node)
		if i == 0 || anchor.After(out.freshest) {
			out.freshest = anchor
		}
		if i == 0 || anchor.Before(out.oldest) {
			out.oldest = anchor
		}
	}
	out.hasRange = !out.freshest.IsZero() && !out.oldest.IsZero() && out.freshest.After(out.oldest)
	return out
}

func temporalAnchor(node models.Node) time.Time {
	if !node.UpdatedAt.IsZero() {
		return node.UpdatedAt
	}
	return node.CreatedAt
}

func temporalNodeScore(node models.Node, originalRank int, profile temporalQueryProfile, rng temporalRange) float64 {
	score := 0.0
	if originalRank >= 0 {
		score += 0.08 / float64(originalRank+1)
	}

	anchor := temporalAnchor(node)
	if rng.hasRange {
		span := rng.freshest.Sub(rng.oldest).Hours()
		if span > 0 {
			position := anchor.Sub(rng.oldest).Hours() / span
			position = math.Max(0, math.Min(1, position))
			if profile.PreferRecent {
				score += position * 0.35
			}
			if profile.PreferHistorical {
				score += (1 - position) * 0.35
			}
		}
	}

	if len(profile.YearHints) > 0 {
		bestYearScore := -0.2
		for _, year := range nodeTemporalYears(node) {
			for _, hint := range profile.YearHints {
				diff := math.Abs(float64(year - hint))
				candidate := 0.0
				switch {
				case diff == 0:
					candidate = 0.45
				case diff == 1:
					candidate = 0.18
				case diff <= 3:
					candidate = 0.08
				default:
					candidate = -math.Min(diff/20, 0.2)
				}
				if candidate > bestYearScore {
					bestYearScore = candidate
				}
			}
		}
		score += bestYearScore
	}

	if score > temporalScoreCap {
		return temporalScoreCap
	}
	if score < -temporalScoreCap {
		return -temporalScoreCap
	}
	return score
}

func nodeTemporalYears(node models.Node) []int {
	text := models.BuildNodeSearchText(&node)
	matches := yearPattern.FindAllString(text, -1)
	seen := map[int]struct{}{}
	out := make([]int, 0, len(matches)+2)
	for _, match := range matches {
		if parsed, err := time.Parse("2006", match); err == nil {
			year := parsed.Year()
			if _, ok := seen[year]; ok {
				continue
			}
			seen[year] = struct{}{}
			out = append(out, year)
		}
	}
	for _, ts := range []time.Time{node.UpdatedAt, node.CreatedAt} {
		if ts.IsZero() {
			continue
		}
		year := ts.UTC().Year()
		if _, ok := seen[year]; ok {
			continue
		}
		seen[year] = struct{}{}
		out = append(out, year)
	}
	return out
}
