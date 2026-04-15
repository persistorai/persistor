package main

import (
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/eval"
)

func TestPrintEvalComparisonTable(t *testing.T) {
	flagFmt = "table"
	got := captureStdout(t, func() {
		printEvalComparisonTable(&eval.ComparisonReport{
			Summary: map[string]eval.ComparisonSummary{
				"term_focus": {
					PassedDelta:       1,
					FailedDelta:       -1,
					RecallAtKDelta:    0.25,
					PrecisionAtKDelta: 0.125,
					Improved:          1,
					ChangedQuestions: []eval.QuestionDelta{{
						Prompt:           "Which project belongs to Brian personally instead of Dirt Road Systems?",
						Category:         "contradiction_handling",
						BaselineOutcome:  "fail",
						CandidateOutcome: "pass",
						BaselineFound:    "matched:- | missed:label:persistor",
						CandidateFound:   "matched:label:persistor | missed:-",
						BaselineTopHit:   "Dirt Road Systems",
						CandidateTopHit:  "Persistor",
						Change:           "improved",
					}},
				},
			},
		})
	})

	for _, want := range []string{"PROFILE", "PASSΔ", "term_focus", "+1", "PROFILE term_focus changed questions", "improved", "Dirt Road Systems -> Persistor"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}
