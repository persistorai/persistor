package service

import (
	"context"
	"math"
	"sort"
	"strings"
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
	ranked := make([]rerankedNode, 0, len(nodes))
	for idx, node := range nodes {
		ranked = append(ranked, rerankedNode{
			node:  node,
			score: prototypeNodeScore(query, terms, node, idx, weights),
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
