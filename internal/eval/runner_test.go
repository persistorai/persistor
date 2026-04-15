package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/persistorai/persistor/client"
)

type fakeSearchClient struct {
	fullText func(context.Context, string, *client.SearchOptions) ([]client.Node, error)
	semantic func(context.Context, string, int) ([]client.ScoredNode, error)
	hybrid   func(context.Context, string, *client.SearchOptions) ([]client.Node, error)
}

func (f fakeSearchClient) FullText(ctx context.Context, query string, opts *client.SearchOptions) ([]client.Node, error) {
	return f.fullText(ctx, query, opts)
}

func (f fakeSearchClient) Semantic(ctx context.Context, query string, limit int) ([]client.ScoredNode, error) {
	return f.semantic(ctx, query, limit)
}

func (f fakeSearchClient) Hybrid(ctx context.Context, query string, opts *client.SearchOptions) ([]client.Node, error) {
	return f.hybrid(ctx, query, opts)
}

func TestRunnerRunPassesWhenExpectedResultIsReturned(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, _ *client.SearchOptions) ([]client.Node, error) {
			return []client.Node{{ID: "big-jerry", Label: "Big Jerry", Type: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Big Jerry?",
			ExpectedNodeIDs: []string{"big-jerry"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Passed != 1 {
		t.Fatalf("expected 1 passed question, got %d", report.Passed)
	}
	if !report.Results[0].Passed {
		t.Fatal("expected question to pass")
	}
}

func TestRunnerRunFailsWhenExpectedResultMissing(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, _ *client.SearchOptions) ([]client.Node, error) {
			return []client.Node{{ID: "yard-rake", Label: "Yard Rake", Type: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Big Jerry?",
			ExpectedNodeIDs: []string{"big-jerry"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Failed != 1 {
		t.Fatalf("expected 1 failed question, got %d", report.Failed)
	}
	if report.Results[0].Passed {
		t.Fatal("expected question to fail")
	}
}

func TestRunnerRunMatchesExpectedLabels(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, _ *client.SearchOptions) ([]client.Node, error) {
			return []client.Node{{ID: "deerprint", Label: "DeerPrint", Type: "project"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:         "What changed in DeerPrint production on Apr 1 and Apr 2?",
			ExpectedLabels: []string{"DeerPrint"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Passed != 1 || !report.Results[0].Passed {
		t.Fatalf("expected label match to pass, got %#v", report.Results[0])
	}
}

func TestRunnerRunTracksCategoryBreakdownAndTopHitPreference(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, query string, _ *client.SearchOptions) ([]client.Node, error) {
			switch query {
			case "What happened on Christmas Eve 2025?":
				return []client.Node{{ID: "christmas-eve-breakthrough", Label: "Christmas Eve Breakthrough", Type: "event"}}, nil
			case "Which project belongs to Brian personally instead of Dirt Road Systems?":
				return []client.Node{
					{ID: "dirt-road-systems", Label: "Dirt Road Systems", Type: "company"},
					{ID: "persistor", Label: "Persistor", Type: "project"},
				}, nil
			default:
				return nil, nil
			}
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{
			{
				Prompt:              "What happened on Christmas Eve 2025?",
				Category:            "temporal_recall",
				ExpectedLabels:      []string{"Christmas Eve Breakthrough"},
				PreferredFirstLabel: "Christmas Eve Breakthrough",
			},
			{
				Prompt:              "Which project belongs to Brian personally instead of Dirt Road Systems?",
				Category:            "file_vs_graph_preference",
				ExpectedLabels:      []string{"Persistor"},
				PreferredFirstLabel: "Persistor",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Passed != 1 || report.Failed != 1 {
		t.Fatalf("expected 1 pass and 1 fail, got %#v", report)
	}
	if len(report.Categories) != 2 {
		t.Fatalf("expected 2 category summaries, got %#v", report.Categories)
	}
	if !report.Results[0].PreferredFirstMatched {
		t.Fatalf("expected first result top-hit preference to pass, got %#v", report.Results[0])
	}
	if report.Results[1].PreferredFirstMatched {
		t.Fatalf("expected second result top-hit preference to fail, got %#v", report.Results[1])
	}
	if report.Results[1].Passed {
		t.Fatalf("expected second result to fail overall, got %#v", report.Results[1])
	}
}

func TestRunnerRunCapturesSearchErrors(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, _ *client.SearchOptions) ([]client.Node, error) {
			return nil, errors.New("search unavailable")
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Big Jerry?",
			ExpectedNodeIDs: []string{"big-jerry"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Results[0].Error == "" {
		t.Fatal("expected question error to be captured")
	}
}

func TestRunnerRunUsesHybridRerankMode(t *testing.T) {
	t.Parallel()

	var opts *client.SearchOptions
	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, got *client.SearchOptions) ([]client.Node, error) {
			opts = got
			return []client.Node{{ID: "big-jerry", Label: "Big Jerry", Type: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:                "Who is Big Jerry?",
			SearchMode:            "hybrid_rerank",
			InternalRerankProfile: "term_focus",
			ExpectedNodeIDs:       []string{"big-jerry"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if report.Passed != 1 {
		t.Fatalf("expected pass, got %#v", report.Results[0])
	}
	if opts == nil || opts.InternalRerank != "prototype" {
		t.Fatalf("expected prototype internal rerank option, got %#v", opts)
	}
	if opts.InternalRerankProfile != "term_focus" {
		t.Fatalf("expected term_focus rerank profile, got %#v", opts)
	}
}

func TestRunnerComparePrototypeProfiles(t *testing.T) {
	t.Parallel()

	calls := make([]string, 0, 4)
	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, opts *client.SearchOptions) ([]client.Node, error) {
			calls = append(calls, opts.InternalRerankProfile)
			return []client.Node{{ID: "big-jerry", Label: "Big Jerry", Type: "animal"}}, nil
		},
	})

	comparison, err := runner.ComparePrototypeProfiles(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Big Jerry?",
			SearchMode:      "hybrid_rerank",
			ExpectedNodeIDs: []string{"big-jerry"},
		}},
	}, []string{"term_focus", "salience_focus"})
	if err != nil {
		t.Fatalf("ComparePrototypeProfiles returned error: %v", err)
	}
	if comparison.Baseline.Passed != 1 {
		t.Fatalf("expected baseline pass, got %#v", comparison.Baseline)
	}
	if len(comparison.Profiles) != 2 {
		t.Fatalf("expected 2 profile reports, got %d", len(comparison.Profiles))
	}
	if len(comparison.Summary) != 2 {
		t.Fatalf("expected 2 profile summaries, got %d", len(comparison.Summary))
	}
	if got := calls[0]; got != "default" {
		t.Fatalf("expected baseline to use default profile, got %q", got)
	}
	if calls[1] != "term_focus" || calls[2] != "salience_focus" {
		t.Fatalf("unexpected profile call order: %v", calls)
	}
}

func TestRunnerComparePrototypeProfilesSummarizesQuestionLevelChanges(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, query string, opts *client.SearchOptions) ([]client.Node, error) {
			switch query {
			case "Which project belongs to Brian personally instead of Dirt Road Systems?":
				if opts.InternalRerankProfile == "term_focus" {
					return []client.Node{{ID: "persistor", Label: "Persistor", Type: "project"}}, nil
				}
				return []client.Node{{ID: "dirt-road-systems", Label: "Dirt Road Systems", Type: "company"}}, nil
			case "Who is Big Jerry?":
				return []client.Node{{ID: "big-jerry", Label: "Big Jerry", Type: "animal"}}, nil
			default:
				return nil, nil
			}
		},
	})

	comparison, err := runner.ComparePrototypeProfiles(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{
			{
				Prompt:              "Which project belongs to Brian personally instead of Dirt Road Systems?",
				SearchMode:          "hybrid_rerank",
				ExpectedLabels:      []string{"Persistor"},
				PreferredFirstLabel: "Persistor",
			},
			{
				Prompt:              "Who is Big Jerry?",
				SearchMode:          "hybrid_rerank",
				ExpectedLabels:      []string{"Big Jerry"},
				PreferredFirstLabel: "Big Jerry",
			},
		},
	}, []string{"term_focus"})
	if err != nil {
		t.Fatalf("ComparePrototypeProfiles returned error: %v", err)
	}

	summary := comparison.Summary["term_focus"]
	if summary.Improved != 1 || summary.Regressed != 0 {
		t.Fatalf("expected one improvement and no regressions, got %#v", summary)
	}
	if len(summary.ChangedQuestions) != 1 {
		t.Fatalf("expected one changed question, got %#v", summary.ChangedQuestions)
	}
	if summary.ChangedQuestions[0].Prompt != "Which project belongs to Brian personally instead of Dirt Road Systems?" {
		t.Fatalf("unexpected changed question: %#v", summary.ChangedQuestions[0])
	}
	if summary.ChangedQuestions[0].Change != "improved" {
		t.Fatalf("expected improved delta, got %#v", summary.ChangedQuestions[0])
	}
}

func TestRunnerComparePrototypeProfilesSummarizesDeltas(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		hybrid: func(_ context.Context, _ string, opts *client.SearchOptions) ([]client.Node, error) {
			if opts.InternalRerankProfile == "term_focus" {
				return []client.Node{{ID: "persistor", Label: "Persistor", Type: "project"}}, nil
			}
			return []client.Node{{ID: "dirt-road-systems", Label: "Dirt Road Systems", Type: "company"}}, nil
		},
	})

	comparison, err := runner.ComparePrototypeProfiles(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:              "Which project belongs to Brian personally instead of Dirt Road Systems?",
			SearchMode:          "hybrid_rerank",
			ExpectedLabels:      []string{"Persistor"},
			PreferredFirstLabel: "Persistor",
		}},
	}, []string{"term_focus"})
	if err != nil {
		t.Fatalf("ComparePrototypeProfiles returned error: %v", err)
	}

	summary := comparison.Summary["term_focus"]
	if summary.PassedDelta != 1 || summary.FailedDelta != -1 {
		t.Fatalf("expected delta summary to show improvement, got %#v", summary)
	}
	if summary.RecallAtKDelta <= 0 || summary.PrecisionAtKDelta <= 0 {
		t.Fatalf("expected positive metric deltas, got %#v", summary)
	}
}
