package eval

import (
	"context"
	"fmt"
	"time"
)

// SearchClient is the retrieval behavior the evaluator needs. Retrieval is
// full-text only, so full-text search is the whole interface.
type SearchClient interface {
	FullText(ctx context.Context, query string, opts *SearchOptions) ([]Node, error)
}

// Runner executes memory evaluation fixtures against a search client.
type Runner struct {
	search SearchClient
}

// NewRunner creates a new evaluation runner.
func NewRunner(search SearchClient) *Runner {
	return &Runner{search: search}
}

const (
	defaultSearchMode = "text"
	defaultLimit      = 5
)

// Run executes the given fixture and returns a report.
func (r *Runner) Run(ctx context.Context, fixture *Fixture) (*Report, error) {
	results := make([]QuestionEval, 0, len(fixture.Questions))
	var passed int
	var totalRecall float64
	var totalPrecision float64
	var totalLatencyMs float64
	byCategory := make(map[string][]QuestionEval)

	for i := range fixture.Questions {
		q := &fixture.Questions[i]
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
		if category := normalizeCategory(q.Category); category != "" {
			byCategory[category] = append(byCategory[category], result)
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
		Categories:       summarizeCategories(byCategory),
		Results:          results,
	}, nil
}

func (r *Runner) runQuestion(ctx context.Context, q *Question) QuestionEval {
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
			Prompt:        q.Prompt,
			Category:      normalizeCategory(q.Category),
			SearchMode:    mode,
			Limit:         limit,
			LatencyMs:     latencyMs,
			ExpectedCount: expectedCount(q),
			Error:         err.Error(),
		}
	}

	matched, missed := scoreReturned(q, returned)
	foundCount := len(matched)
	expected := len(matched) + len(missed)
	preferredFirst := preferredFirstExpectation(q)
	preferredFirstMatched := preferredFirst == "" || matchesExpectation(returned, preferredFirst)
	passed := expected > 0 && foundCount == expected && preferredFirstMatched

	return QuestionEval{
		Prompt:                    q.Prompt,
		Category:                  normalizeCategory(q.Category),
		SearchMode:                mode,
		Limit:                     limit,
		Passed:                    passed,
		LatencyMs:                 latencyMs,
		FoundExpectedCount:        foundCount,
		ExpectedCount:             expected,
		ReturnedCount:             len(returned),
		ExpectedMatches:           matched,
		MissedExpectations:        missed,
		PreferredFirstExpectation: preferredFirst,
		PreferredFirstMatched:     preferredFirst != "" && preferredFirstMatched,
		Returned:                  returned,
	}
}

func (r *Runner) searchOnce(ctx context.Context, mode, prompt string, limit int) ([]ReturnedResult, error) {
	switch mode {
	case "text":
		nodes, err := r.search.FullText(ctx, prompt, &SearchOptions{Limit: limit})
		if err != nil {
			return nil, err
		}
		return mapNodes(nodes), nil
	default:
		return nil, fmt.Errorf("unsupported search mode %q (this evaluator is text-only)", mode)
	}
}
