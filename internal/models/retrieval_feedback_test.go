package models

import "testing"

func TestRetrievalFeedbackRequestValidate(t *testing.T) {
	req := RetrievalFeedbackRequest{Query: "test", Outcome: RetrievalOutcomeHelpful}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if err := (RetrievalFeedbackRequest{Outcome: RetrievalOutcomeHelpful}).Validate(); err == nil {
		t.Fatal("expected missing query error")
	}
	if err := (RetrievalFeedbackRequest{Query: "test", Outcome: "maybe"}).Validate(); err == nil {
		t.Fatal("expected invalid outcome error")
	}
}

func TestDeriveRetrievalSignals(t *testing.T) {
	tests := []struct {
		name string
		req  RetrievalFeedbackRequest
		want []string
	}{
		{name: "helpful selected result", req: RetrievalFeedbackRequest{Query: "q", Outcome: RetrievalOutcomeHelpful, RetrievedNodeIDs: []string{"n1", "n2"}, SelectedNodeIDs: []string{"n2"}}, want: []string{RetrievalSignalConfirmedRecall}},
		{name: "unhelpful result", req: RetrievalFeedbackRequest{Query: "q", Outcome: RetrievalOutcomeUnhelpful, RetrievedNodeIDs: []string{"n1"}}, want: []string{RetrievalSignalIrrelevantResult}},
		{name: "missed known item and empty", req: RetrievalFeedbackRequest{Query: "q", Outcome: RetrievalOutcomeMissed, ExpectedNodeIDs: []string{"n9"}}, want: []string{RetrievalSignalEmptyResult, RetrievalSignalMissedKnownItem}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveRetrievalSignals(tc.req)
			if len(got) != len(tc.want) {
				t.Fatalf("signals = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("signals = %#v, want %#v", got, tc.want)
				}
			}
		})
	}
}
