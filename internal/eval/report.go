package eval

import "slices"

// Report summarizes evaluation results for a fixture.
type Report struct {
	FixtureName      string           `json:"fixture_name"`
	QuestionCount    int              `json:"question_count"`
	Passed           int              `json:"passed"`
	Failed           int              `json:"failed"`
	RecallAtK        float64          `json:"recall_at_k"`
	PrecisionAtK     float64          `json:"precision_at_k"`
	AverageLatencyMs float64          `json:"average_latency_ms"`
	Categories       []CategoryReport `json:"categories,omitempty"`
	Results          []QuestionEval   `json:"results"`
}

// CategoryReport summarizes results for one evaluation category.
type CategoryReport struct {
	Name             string  `json:"name"`
	QuestionCount    int     `json:"question_count"`
	Passed           int     `json:"passed"`
	Failed           int     `json:"failed"`
	RecallAtK        float64 `json:"recall_at_k"`
	PrecisionAtK     float64 `json:"precision_at_k"`
	AverageLatencyMs float64 `json:"average_latency_ms"`
}

// QuestionEval contains the result of evaluating one question.
type QuestionEval struct {
	Prompt                    string           `json:"prompt"`
	Category                  string           `json:"category,omitempty"`
	SearchMode                string           `json:"search_mode"`
	Limit                     int              `json:"limit"`
	Passed                    bool             `json:"passed"`
	LatencyMs                 float64          `json:"latency_ms"`
	FoundExpectedCount        int              `json:"found_expected_count"`
	ExpectedCount             int              `json:"expected_count"`
	ReturnedCount             int              `json:"returned_count"`
	ExpectedMatches           []string         `json:"expected_matches,omitempty"`
	MissedExpectations        []string         `json:"missed_expectations,omitempty"`
	PreferredFirstExpectation string           `json:"preferred_first_expectation,omitempty"`
	PreferredFirstMatched     bool             `json:"preferred_first_matched,omitempty"`
	Returned                  []ReturnedResult `json:"returned"`
	Error                     string           `json:"error,omitempty"`
}

// ReturnedResult is a compact representation of a returned search hit.
type ReturnedResult struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// summarizeCategories aggregates per-category recall/precision/latency from the
// grouped question results, ordered by category name for stable output.
func summarizeCategories(byCategory map[string][]QuestionEval) []CategoryReport {
	if len(byCategory) == 0 {
		return nil
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	reports := make([]CategoryReport, 0, len(categories))
	for _, category := range categories {
		results := byCategory[category]
		var passed int
		var recall float64
		var precision float64
		var latency float64
		for i := range results {
			result := &results[i]
			if result.Passed {
				passed++
			}
			latency += result.LatencyMs
			if result.ExpectedCount > 0 {
				recall += float64(result.FoundExpectedCount) / float64(result.ExpectedCount)
			}
			if result.ReturnedCount > 0 {
				precision += float64(result.FoundExpectedCount) / float64(result.ReturnedCount)
			}
		}
		count := len(results)
		reports = append(reports, CategoryReport{
			Name:             category,
			QuestionCount:    count,
			Passed:           passed,
			Failed:           count - passed,
			RecallAtK:        recall / float64(count),
			PrecisionAtK:     precision / float64(count),
			AverageLatencyMs: latency / float64(count),
		})
	}
	return reports
}
