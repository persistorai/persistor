package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/persistorai/persistor/internal/models"
)

const (
	internalRerankPrototype   = "prototype"
	rerankCandidateMultiplier = 3
	maxRerankCandidates       = 50
)

type rerankContextKey string

const (
	rerankModeContextKey    rerankContextKey = "internal_rerank_mode"
	rerankProfileContextKey rerankContextKey = "internal_rerank_profile"
)

type prototypeRerankWeights struct {
	ExactLabel       float64
	ContainsQuery    float64
	LabelTerm        float64
	SearchTextTerm   float64
	TermCoverage     float64
	UserBoosted      float64
	SalienceDivisor  float64
	SalienceCap      float64
	OriginalRankBias float64
}

var prototypeRerankProfiles = map[string]prototypeRerankWeights{
	"default": {
		ExactLabel:       10,
		ContainsQuery:    4,
		LabelTerm:        2.5,
		SearchTextTerm:   1.2,
		TermCoverage:     3,
		UserBoosted:      0.4,
		SalienceDivisor:  100,
		SalienceCap:      1.5,
		OriginalRankBias: 1,
	},
	"term_focus": {
		ExactLabel:       10,
		ContainsQuery:    4,
		LabelTerm:        3.2,
		SearchTextTerm:   1.8,
		TermCoverage:     4.2,
		UserBoosted:      0.3,
		SalienceDivisor:  140,
		SalienceCap:      1.0,
		OriginalRankBias: 0.8,
	},
	"salience_focus": {
		ExactLabel:       9,
		ContainsQuery:    3.5,
		LabelTerm:        2.1,
		SearchTextTerm:   1,
		TermCoverage:     2.5,
		UserBoosted:      0.5,
		SalienceDivisor:  70,
		SalienceCap:      2.2,
		OriginalRankBias: 1.1,
	},
}

// WithInternalRerankMode attaches an internal-only reranking mode to the context.
func WithInternalRerankMode(ctx context.Context, mode string) context.Context {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return ctx
	}
	return context.WithValue(ctx, rerankModeContextKey, mode)
}

// InternalRerankMode returns the normalized internal-only rerank mode from context.
func InternalRerankMode(ctx context.Context) string {
	mode, _ := ctx.Value(rerankModeContextKey).(string)
	return strings.TrimSpace(strings.ToLower(mode))
}

// WithInternalRerankProfile attaches an internal-only rerank weighting profile to the context.
func WithInternalRerankProfile(ctx context.Context, profile string) context.Context {
	profile = normalizePrototypeRerankProfile(profile)
	if profile == "" {
		return ctx
	}
	return context.WithValue(ctx, rerankProfileContextKey, profile)
}

// InternalRerankProfile returns the normalized internal-only rerank profile from context.
func InternalRerankProfile(ctx context.Context) string {
	profile, _ := ctx.Value(rerankProfileContextKey).(string)
	return normalizePrototypeRerankProfile(profile)
}

func shouldPrototypeRerank(ctx context.Context, limit int) bool {
	return limit > 0 && InternalRerankMode(ctx) == internalRerankPrototype
}

func rerankCandidateLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	candidateLimit := limit * rerankCandidateMultiplier
	if candidateLimit < limit {
		candidateLimit = limit
	}
	if candidateLimit > maxRerankCandidates {
		candidateLimit = maxRerankCandidates
	}
	return candidateLimit
}

type rerankedNode struct {
	node  models.Node
	score float64
}

type beliefAwareWeights struct {
	SupportedStatus     float64
	ContestedPenalty    float64
	SupersededPenalty   float64
	PreferredCurrent    float64
	PreferredConfidence float64
	EvidenceCount       float64
	RecentClaim         float64
	RecencyWindowDays   float64
	ScoreCap            float64
	OriginalRankBias    float64
}

var defaultBeliefAwareWeights = beliefAwareWeights{
	SupportedStatus:     0.32,
	ContestedPenalty:    0.18,
	SupersededPenalty:   0.3,
	PreferredCurrent:    0.08,
	PreferredConfidence: 0.08,
	EvidenceCount:       0.05,
	RecentClaim:         0.08,
	RecencyWindowDays:   365,
	ScoreCap:            0.8,
	OriginalRankBias:    0.35,
}

func prototypeRerankNodes(query string, nodes []models.Node, limit int) []models.Node {
	return prototypeRerankNodesWithProfile(query, nodes, limit, "default")
}

func prototypeRerankNodesWithProfile(query string, nodes []models.Node, limit int, profile string) []models.Node {
	if len(nodes) == 0 || limit <= 0 {
		return nodes
	}

	terms := tokenizeSearchText(query)
	if len(terms) == 0 {
		if len(nodes) > limit {
			return nodes[:limit]
		}
		return nodes
	}

	weights := prototypeWeightsForProfile(profile)
	freshest := freshestBeliefObservation(nodes)
	ranked := make([]rerankedNode, 0, len(nodes))
	for idx, node := range nodes {
		ranked = append(ranked, rerankedNode{
			node:  node,
			score: prototypeNodeScore(query, terms, node, idx, weights) + beliefAwareNodeScore(node, idx, freshest, defaultBeliefAwareWeights),
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

func shapeBeliefAwareNodes(nodes []models.Node, limit int) []models.Node {
	if len(nodes) == 0 || limit <= 0 {
		return nodes
	}

	freshest := freshestBeliefObservation(nodes)
	ranked := make([]rerankedNode, 0, len(nodes))
	for idx, node := range nodes {
		ranked = append(ranked, rerankedNode{node: node, score: beliefAwareNodeScore(node, idx, freshest, defaultBeliefAwareWeights)})
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

func beliefAwareNodeScore(node models.Node, originalRank int, freshest *time.Time, weights beliefAwareWeights) float64 {
	score := 0.0
	if weights.OriginalRankBias > 0 {
		score += weights.OriginalRankBias / float64(originalRank+1)
	}

	for property, belief := range decodeBeliefStates(node.Properties) {
		switch belief.Status {
		case models.FactBeliefStatusSupported:
			score += weights.SupportedStatus
		case models.FactBeliefStatusContested:
			score -= weights.ContestedPenalty
		case models.FactBeliefStatusSuperseded:
			score -= weights.SupersededPenalty
		}
		score += math.Min(float64(belief.EvidenceCount), 4) * weights.EvidenceCount
		score += math.Min(math.Max(belief.PreferredConfidence, 0), 1) * weights.PreferredConfidence
		if preferredMatchesProperty(node.Properties, property, belief.PreferredValue) {
			score += weights.PreferredCurrent
		}
		if recent := parseBeliefTime(latestBeliefObservation(belief)); recent != nil && freshest != nil && weights.RecencyWindowDays > 0 {
			ageDays := freshest.Sub(*recent).Hours() / 24
			if ageDays < 0 {
				ageDays = 0
			}
			freshness := math.Max(0, 1-(ageDays/weights.RecencyWindowDays))
			score += freshness * weights.RecentClaim
		}
	}

	if score > weights.ScoreCap {
		return weights.ScoreCap
	}
	if score < -weights.ScoreCap {
		return -weights.ScoreCap
	}
	return score
}

func decodeBeliefStates(props map[string]any) map[string]models.FactBeliefState {
	raw, ok := props[models.FactBeliefsProperty]
	if !ok {
		return map[string]models.FactBeliefState{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return map[string]models.FactBeliefState{}
	}
	var out map[string]models.FactBeliefState
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return map[string]models.FactBeliefState{}
	}
	return out
}

func freshestBeliefObservation(nodes []models.Node) *time.Time {
	var freshest *time.Time
	for _, node := range nodes {
		for _, belief := range decodeBeliefStates(node.Properties) {
			if observed := parseBeliefTime(latestBeliefObservation(belief)); observed != nil {
				if freshest == nil || observed.After(*freshest) {
					copyObserved := *observed
					freshest = &copyObserved
				}
			}
		}
	}
	return freshest
}

func latestBeliefObservation(belief models.FactBeliefState) string {
	latest := ""
	for _, claim := range belief.Claims {
		if claim.LastObservedAt > latest {
			latest = claim.LastObservedAt
		}
	}
	return latest
}

func parseBeliefTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed
		}
	}
	return nil
}

func preferredMatchesProperty(props map[string]any, property string, preferred any) bool {
	current, ok := props[property]
	return ok && canonicalBeliefValue(current) == canonicalBeliefValue(preferred)
}

func canonicalBeliefValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func prototypeNodeScore(query string, terms []string, node models.Node, originalRank int, weights prototypeRerankWeights) float64 {
	label := strings.ToLower(strings.TrimSpace(node.Label))
	query = strings.ToLower(strings.TrimSpace(query))
	searchText := strings.ToLower(models.BuildNodeSearchText(&node))

	var score float64
	if label == query {
		score += weights.ExactLabel
	}
	if strings.Contains(label, query) {
		score += weights.ContainsQuery
	}

	matchedTerms := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(label, term) {
			score += weights.LabelTerm
			matchedTerms++
			continue
		}
		if strings.Contains(searchText, term) {
			score += weights.SearchTextTerm
			matchedTerms++
		}
	}

	if len(terms) > 0 {
		score += weights.TermCoverage * (float64(matchedTerms) / float64(len(terms)))
	}
	if node.UserBoosted {
		score += weights.UserBoosted
	}
	if weights.SalienceDivisor > 0 {
		score += math.Min(node.Salience/weights.SalienceDivisor, weights.SalienceCap)
	}
	if weights.OriginalRankBias > 0 {
		score += weights.OriginalRankBias / float64(originalRank+1)
	}
	return score
}

func normalizePrototypeRerankProfile(profile string) string {
	profile = strings.TrimSpace(strings.ToLower(profile))
	if profile == "" {
		return ""
	}
	if _, ok := prototypeRerankProfiles[profile]; ok {
		return profile
	}
	return "default"
}

func prototypeWeightsForProfile(profile string) prototypeRerankWeights {
	profile = normalizePrototypeRerankProfile(profile)
	if profile == "" {
		profile = "default"
	}
	return prototypeRerankProfiles[profile]
}

func tokenizeSearchText(text string) []string {
	parts := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	terms := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}
