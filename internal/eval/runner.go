package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/persistorai/persistor/client"
)

// SearchClient is the subset of client search behavior needed by the evaluator.
type SearchClient interface {
	FullText(ctx context.Context, query string, opts *client.SearchOptions) ([]client.Node, error)
	Semantic(ctx context.Context, query string, limit int) ([]client.ScoredNode, error)
	Hybrid(ctx context.Context, query string, opts *client.SearchOptions) ([]client.Node, error)
}

// Runner executes memory evaluation fixtures against a search client.
type Runner struct {
	search SearchClient
}

// NewRunner creates a new evaluation runner.
func NewRunner(search SearchClient) *Runner {
	return &Runner{search: search}
}

// Report summarizes evaluation results for a fixture.
type Report struct {
	FixtureName        string         `json:"fixture_name"`
	QuestionCount      int            `json:"question_count"`
	Passed             int            `json:"passed"`
	Failed             int            `json:"failed"`
	RecallAtK          float64        `json:"recall_at_k"`
	PrecisionAtK       float64        `json:"precision_at_k"`
	AverageLatencyMs   float64        `json:"average_latency_ms"`
	Results            []QuestionEval `json:"results"`
}

// QuestionEval contains the result of evaluating one question.
type QuestionEval struct {
	Prompt             string            `json:"prompt"`
	SearchMode         string            `json:"search_mode"`
	Limit              int               `json:"limit"`
	Passed             bool              `json:"passed"`
	LatencyMs          float64           `json:"latency_ms"`
	FoundExpectedCount int               `json:"found_expected_count"`
	ExpectedCount      int               `json:"expected_count"`
	ReturnedCount      int               `json:"returned_count"`
	ExpectedMatches    []string          `json:"expected_matches,omitempty"`
	MissedExpectations []string          `json:"missed_expectations,omitempty"`
	Returned           []ReturnedResult  `json:"returned"`
	Error              string            `json:"error,omitempty"`
}

// ReturnedResult is a compact representation of a returned search hit.
type ReturnedResult struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Type  string  `json:"type"`
	Score float64 `json:"score,omitempty"`
}

const (
	defaultSearchMode = "hybrid"
	defaultLimit      = 5
)

// Run executes the given fixture and returns a report.
func (r *Runner) Run(ctx context.Context, fixture *Fixture) (*Report, error) {
	results := make([]QuestionEval, 0, len(fixture.Questions))
	var passed int
	var totalRecall float64
	var totalPrecision float64
	var totalLatencyMs float64

	for _, q := range fixture.Questions {
		result := r.runQuestion(ctx, q)
		results = append(results, result)
		if result.Passed {
			passed++
		}
		totalLatencyMs += result.LatencyMs
		if result.ExpectedCount > 0 {
			totalRecall += float64(result.FoundExpectedCount) / float64(result.ExpectedCount)
		}
		if result.ReturnedCount > 0 {
			totalPrecision += float64(result.FoundExpectedCount) / float64(result.ReturnedCount)
		}
	}

	questionCount := len(fixture.Questions)
	if questionCount == 0 {
		return nil, fmt.Errorf("fixture has no questions")
	}

	return &Report{
		FixtureName:      fixture.Name,
		QuestionCount:    questionCount,
		Passed:           passed,
		Failed:           questionCount - passed,
		RecallAtK:        totalRecall / float64(questionCount),
		PrecisionAtK:     totalPrecision / float64(questionCount),
		AverageLatencyMs: totalLatencyMs / float64(questionCount),
		Results:          results,
	}, nil
}

func (r *Runner) runQuestion(ctx context.Context, q Question) QuestionEval {
	mode := q.SearchMode
	if mode == "" {
		mode = defaultSearchMode
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	started := time.Now()
	returned, err := r.searchOnce(ctx, mode, q.Prompt, limit)
	latencyMs := float64(time.Since(started).Milliseconds())
	if err != nil {
		return QuestionEval{
			Prompt:      q.Prompt,
			SearchMode:  mode,
			Limit:       limit,
			LatencyMs:   latencyMs,
			ExpectedCount: expectedCount(q),
			Error:       err.Error(),
		}
	}

	expectedSet := buildExpectedSet(q)
	matched := make([]string, 0, len(expectedSet))
	missed := make([]string, 0, len(expectedSet))
	seen := make(map[string]bool, len(expectedSet))

	for _, item := range returned {
		for _, key := range expectedKeysForResult(item) {
			if _, ok := expectedSet[key]; ok {
				seen[key] = true
			}
		}
	}

	for key := range expectedSet {
		if seen[key] {
			matched = append(matched, key)
		} else {
			missed = append(missed, key)
		}
	}

	foundCount := len(matched)
	expectedCount := len(expectedSet)
	passed := expectedCount > 0 && foundCount == expectedCount

	return QuestionEval{
		Prompt:             q.Prompt,
		SearchMode:         mode,
		Limit:              limit,
		Passed:             passed,
		LatencyMs:          latencyMs,
		FoundExpectedCount: foundCount,
		ExpectedCount:      expectedCount,
		ReturnedCount:      len(returned),
		ExpectedMatches:    matched,
		MissedExpectations: missed,
		Returned:           returned,
	}
}

func (r *Runner) searchOnce(ctx context.Context, mode, prompt string, limit int) ([]ReturnedResult, error) {
	switch mode {
	case "text":
		nodes, err := r.search.FullText(ctx, prompt, &client.SearchOptions{Limit: limit})
		if err != nil {
			return nil, err
		}
		return mapNodes(nodes), nil
	case "vector":
		nodes, err := r.search.Semantic(ctx, prompt, limit)
		if err != nil {
			return nil, err
		}
		return mapScoredNodes(nodes), nil
	case "hybrid":
		nodes, err := r.search.Hybrid(ctx, prompt, &client.SearchOptions{Limit: limit})
		if err != nil {
			return nil, err
		}
		return mapNodes(nodes), nil
	default:
		return nil, fmt.Errorf("unsupported search mode %q", mode)
	}
}

func mapNodes(nodes []client.Node) []ReturnedResult {
	results := make([]ReturnedResult, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, ReturnedResult{
			ID:    node.ID,
			Label: node.Label,
			Type:  node.Type,
		})
	}
	return results
}

func mapScoredNodes(nodes []client.ScoredNode) []ReturnedResult {
	results := make([]ReturnedResult, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, ReturnedResult{
			ID:    node.ID,
			Label: node.Label,
			Type:  node.Type,
			Score: node.Score,
		})
	}
	return results
}

func buildExpectedSet(q Question) map[string]struct{} {
	expected := make(map[string]struct{}, len(q.ExpectedNodeIDs)+len(q.ExpectedLabels))
	for _, id := range q.ExpectedNodeIDs {
		expected[normalizeExpected("id", id)] = struct{}{}
	}
	for _, label := range q.ExpectedLabels {
		expected[normalizeExpected("label", label)] = struct{}{}
	}
	return expected
}

func expectedKeysForResult(result ReturnedResult) []string {
	keys := []string{normalizeExpected("id", result.ID)}
	if result.Label != "" {
		keys = append(keys, normalizeExpected("label", result.Label))
	}
	return keys
}

func normalizeExpected(kind, value string) string {
	return kind + ":" + strings.ToLower(strings.TrimSpace(value))
}

func expectedCount(q Question) int {
	return len(buildExpectedSet(q))
}
