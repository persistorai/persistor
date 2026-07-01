package eval

import (
	"context"
	"errors"
	"testing"
)

// fakeSearchClient is a test double for the FTS-only SearchClient.
type fakeSearchClient struct {
	fullText func(context.Context, string, *SearchOptions) ([]NoteResult, error)
}

func (f fakeSearchClient) FullText(ctx context.Context, query string, opts *SearchOptions) ([]NoteResult, error) {
	return f.fullText(ctx, query, opts)
}

func TestRunnerRunPassesWhenExpectedResultIsReturned(t *testing.T) {
	t.Parallel()

	runner := NewRunner(fakeSearchClient{
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]NoteResult, error) {
			return []NoteResult{{ID: "comet", Title: "Comet", Kind: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNoteIDs: []string{"comet"},
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]NoteResult, error) {
			return []NoteResult{{ID: "yard-rake", Title: "Yard Rake", Kind: "animal"}}, nil
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNoteIDs: []string{"comet"},
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]NoteResult, error) {
			return []NoteResult{{ID: "aurora", Title: "Aurora", Kind: "project"}}, nil
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
		fullText: func(_ context.Context, query string, _ *SearchOptions) ([]NoteResult, error) {
			switch query {
			case "What happened on Christmas Eve 2025?":
				return []NoteResult{{ID: "christmas-eve-breakthrough", Title: "Christmas Eve Breakthrough", Kind: "event"}}, nil
			case "Which project belongs to Avery personally instead of Acme Systems?":
				return []NoteResult{
					{ID: "acme-systems", Title: "Acme Systems", Kind: "company"},
					{ID: "persistor", Title: "Persistor", Kind: "project"},
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
		fullText: func(_ context.Context, _ string, _ *SearchOptions) ([]NoteResult, error) {
			return nil, errors.New("search unavailable")
		},
	})

	report, err := runner.Run(context.Background(), &Fixture{
		Name: "memory-fixture",
		Questions: []Question{{
			Prompt:          "Who is Comet?",
			ExpectedNoteIDs: []string{"comet"},
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if report.Results[0].Error == "" {
		t.Fatal("expected question error to be captured")
	}
}

// stubSearch returns a fixed result set for any query.
type stubSearch struct{ notes []NoteResult }

func (s *stubSearch) FullText(context.Context, string, *SearchOptions) ([]NoteResult, error) {
	return s.notes, nil
}

// TestRunAbstainQuestions locks the abstention semantics: zero results = pass,
// any result = fail, scored as one synthetic expectation so aggregation works.
func TestRunAbstainQuestions(t *testing.T) {
	fixture := &Fixture{Name: "abstain", Questions: []Question{
		{Prompt: "nothing about this exists", Category: "abstention", ExpectAbstain: true},
	}}

	silent, err := NewRunner(&stubSearch{}).Run(context.Background(), fixture)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if silent.Passed != 1 || silent.RecallAtK != 1.0 {
		t.Errorf("silence should pass an abstain question: passed=%d recall=%.2f", silent.Passed, silent.RecallAtK)
	}

	noisy, err := NewRunner(&stubSearch{notes: []NoteResult{{ID: "x", Title: "Noise"}}}).
		Run(context.Background(), fixture)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if noisy.Passed != 0 || noisy.RecallAtK != 0 {
		t.Errorf("noise must fail an abstain question: passed=%d recall=%.2f", noisy.Passed, noisy.RecallAtK)
	}
}

// TestFixtureRejectsAbstainWithExpectations locks the validation rule.
func TestFixtureRejectsAbstainWithExpectations(t *testing.T) {
	_, err := parseFixture([]byte(`{"name":"bad","questions":[
		{"prompt":"p","expect_abstain":true,"expected_note_ids":["x"]}]}`))
	if err == nil {
		t.Fatal("fixture combining expect_abstain with expected ids must be rejected")
	}
}
