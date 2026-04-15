package eval

import (
	"context"
	"strings"
)

// ComparisonReport captures baseline and profile-specific evaluation reports.
type ComparisonReport struct {
	Baseline Report                       `json:"baseline"`
	Profiles map[string]Report            `json:"profiles,omitempty"`
	Summary  map[string]ComparisonSummary `json:"summary,omitempty"`
}

// ComparisonSummary captures bounded metric deltas versus the baseline report.
type ComparisonSummary struct {
	PassedDelta       int             `json:"passed_delta"`
	FailedDelta       int             `json:"failed_delta"`
	RecallAtKDelta    float64         `json:"recall_at_k_delta"`
	PrecisionAtKDelta float64         `json:"precision_at_k_delta"`
	Improved          int             `json:"improved"`
	Regressed         int             `json:"regressed"`
	ChangedQuestions  []QuestionDelta `json:"changed_questions,omitempty"`
}

// QuestionDelta captures a question-level change between baseline and candidate runs.
type QuestionDelta struct {
	Prompt           string `json:"prompt"`
	Category         string `json:"category,omitempty"`
	BaselineOutcome  string `json:"baseline_outcome"`
	CandidateOutcome string `json:"candidate_outcome"`
	BaselineTopHit   string `json:"baseline_top_hit,omitempty"`
	CandidateTopHit  string `json:"candidate_top_hit,omitempty"`
	BaselineFound    string `json:"baseline_found"`
	CandidateFound   string `json:"candidate_found"`
	Change           string `json:"change"`
}

// ComparePrototypeProfiles runs the fixture once with the default rerank profile,
// then once per named profile using the internal prototype reranker.
func (r *Runner) ComparePrototypeProfiles(ctx context.Context, fixture *Fixture, profiles []string) (*ComparisonReport, error) {
	baselineFixture := fixture.Clone()
	baselineFixture.ApplyPrototypeRerankProfile("default")

	baseline, err := r.Run(ctx, baselineFixture)
	if err != nil {
		return nil, err
	}

	report := &ComparisonReport{
		Baseline: *baseline,
		Profiles: make(map[string]Report),
		Summary:  make(map[string]ComparisonSummary),
	}
	for _, profile := range profiles {
		profile = strings.TrimSpace(strings.ToLower(profile))
		if profile == "" || profile == "default" {
			continue
		}
		candidate := fixture.Clone()
		candidate.ApplyPrototypeRerankProfile(profile)
		candidateReport, err := r.Run(ctx, candidate)
		if err != nil {
			return nil, err
		}
		changed := summarizeQuestionDeltas(baseline.Results, candidateReport.Results)
		var improved, regressed int
		for _, delta := range changed {
			switch delta.Change {
			case "improved":
				improved++
			case "regressed":
				regressed++
			}
		}
		report.Profiles[profile] = *candidateReport
		report.Summary[profile] = ComparisonSummary{
			PassedDelta:       candidateReport.Passed - baseline.Passed,
			FailedDelta:       candidateReport.Failed - baseline.Failed,
			RecallAtKDelta:    candidateReport.RecallAtK - baseline.RecallAtK,
			PrecisionAtKDelta: candidateReport.PrecisionAtK - baseline.PrecisionAtK,
			Improved:          improved,
			Regressed:         regressed,
			ChangedQuestions:  changed,
		}
	}
	return report, nil
}

func summarizeQuestionDeltas(baseline, candidate []QuestionEval) []QuestionDelta {
	count := len(baseline)
	if len(candidate) < count {
		count = len(candidate)
	}
	changed := make([]QuestionDelta, 0)
	for i := 0; i < count; i++ {
		base := baseline[i]
		cand := candidate[i]
		if !questionChanged(base, cand) {
			continue
		}
		changed = append(changed, QuestionDelta{
			Prompt:           cand.Prompt,
			Category:         cand.Category,
			BaselineOutcome:  questionOutcome(base),
			CandidateOutcome: questionOutcome(cand),
			BaselineTopHit:   topHitLabel(base.Returned),
			CandidateTopHit:  topHitLabel(cand.Returned),
			BaselineFound:    foundSummary(base),
			CandidateFound:   foundSummary(cand),
			Change:           classifyQuestionChange(base, cand),
		})
	}
	return changed
}

func questionChanged(base, cand QuestionEval) bool {
	return base.Passed != cand.Passed ||
		base.FoundExpectedCount != cand.FoundExpectedCount ||
		base.PreferredFirstMatched != cand.PreferredFirstMatched ||
		topHitLabel(base.Returned) != topHitLabel(cand.Returned) ||
		base.Error != cand.Error
}

func classifyQuestionChange(base, cand QuestionEval) string {
	switch {
	case !base.Passed && cand.Passed:
		return "improved"
	case base.Passed && !cand.Passed:
		return "regressed"
	case cand.FoundExpectedCount > base.FoundExpectedCount:
		return "improved"
	case cand.FoundExpectedCount < base.FoundExpectedCount:
		return "regressed"
	default:
		return "changed"
	}
}

func questionOutcome(q QuestionEval) string {
	if q.Error != "" {
		return "error"
	}
	if q.Passed {
		return "pass"
	}
	return "fail"
}

func topHitLabel(returned []ReturnedResult) string {
	if len(returned) == 0 {
		return "-"
	}
	if returned[0].Label != "" {
		return returned[0].Label
	}
	if returned[0].ID != "" {
		return returned[0].ID
	}
	return "-"
}

func foundSummary(q QuestionEval) string {
	matched := "matched:-"
	if len(q.ExpectedMatches) > 0 {
		matched = "matched:" + strings.Join(q.ExpectedMatches, ", ")
	}
	return matched + " | " + missingSummary(q)
}

func missingSummary(q QuestionEval) string {
	if len(q.MissedExpectations) == 0 {
		return "missed:-"
	}
	return "missed:" + strings.Join(q.MissedExpectations, ", ")
}
