package eval

import (
	"context"
	"strings"
)

// ComparisonReport captures baseline and profile-specific evaluation reports.
type ComparisonReport struct {
	Baseline Report            `json:"baseline"`
	Profiles map[string]Report `json:"profiles,omitempty"`
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
	}
	return report, nil
}
