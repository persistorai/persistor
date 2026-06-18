package eval

import (
	"context"
	"errors"
	"testing"
)

// fakeSearchClient is a test double for the FTS-only SearchClient.
type fakeSearchClient struct {
	fullText func(context.Context, string, *SearchOptions) ([]Node, error)
}

func (f fakeSearchClient) FullText(ctx context.Context, query string, opts *SearchOptions) ([]Node, error) {
	return f.fullText(ctx, query, opts)
}

func TestRunnerRunPassesWhenExpectedResultIsReturned(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]Node, error) {
			return []Node{{ID: "comet", Label: "Comet", Type: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNodeIDs: []string{"comet"},
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]Node, error) {
			return []Node{{ID: "yard-rake", Label: "Yard Rake", Type: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNodeIDs: []string{"comet"},
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]Node, error) {
			return []Node{{ID: "aurora", Label: "Aurora", Type: "project"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:         "What changed in Aurora production on Apr 1 and Apr 2?",
			ExpectedLabels: []string{"Aurora"},
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
		fullText: func(_ context.Context, query string, _ *SearchOptions) ([]Node, error) {
			switch query {
			case "What happened on Christmas Eve 2025?":
				return []Node{{ID: "christmas-eve-breakthrough", Label: "Christmas Eve Breakthrough", Type: "event"}}, nil
			case "Which project belongs to Avery personally instead of Acme Systems?":
				return []Node{
					{ID: "acme-systems", Label: "Acme Systems", Type: "company"},
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
				Prompt:              "Which project belongs to Avery personally instead of Acme Systems?",
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]Node, error) {
			return nil, errors.New("search unavailable")
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNodeIDs: []string{"comet"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Results[0].Error == "" {
		t.Fatal("expected question error to be captured")
	}
}
