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
	PassedDelta       int     `json:"passed_delta"`
	FailedDelta       int     `json:"failed_delta"`
	RecallAtKDelta    float64 `json:"recall_at_k_delta"`
	PrecisionAtKDelta float64 `json:"precision_at_k_delta"`
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
		report.Profiles[profile] = *candidateReport
		report.Summary[profile] = ComparisonSummary{
			PassedDelta:       candidateReport.Passed - baseline.Passed,
			FailedDelta:       candidateReport.Failed - baseline.Failed,
			RecallAtKDelta:    candidateReport.RecallAtK - baseline.RecallAtK,
			PrecisionAtKDelta: candidateReport.PrecisionAtK - baseline.PrecisionAtK,
		}
	}
	return report, nil
}
