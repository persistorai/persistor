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
