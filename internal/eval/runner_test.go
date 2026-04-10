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
	if got := calls[0]; got != "default" {
		t.Fatalf("expected baseline to use default profile, got %q", got)
	}
	if calls[1] != "term_focus" || calls[2] != "salience_focus" {
		t.Fatalf("unexpected profile call order: %v", calls)
	}
}
