package service

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestRecordRetrievalFeedbackDerivesIntentAndSignals(t *testing.T) {
	store := &mockAdminStore{}
	svc := NewAdminService(store, nil, logrus.New())

	record, err := svc.RecordRetrievalFeedback(context.Background(), "tenant", models.RetrievalFeedbackRequest{
		Query:            "Who is Big Jerry?",
		SearchMode:       "hybrid",
		Outcome:          models.RetrievalOutcomeHelpful,
		RetrievedNodeIDs: []string{"node-1", "node-2"},
		SelectedNodeIDs:  []string{"node-2"},
	})
	if err != nil {
		t.Fatalf("RecordRetrievalFeedback: %v", err)
	}
	if record.Intent != string(SearchIntentEntity) {
		t.Fatalf("intent = %q, want %q", record.Intent, SearchIntentEntity)
	}
	if len(record.Signals) != 1 || record.Signals[0] != models.RetrievalSignalConfirmedRecall {
		t.Fatalf("signals = %#v", record.Signals)
	}
}

func TestGetRetrievalFeedbackSummaryAggregatesByQuery(t *testing.T) {
	now := time.Now()
	store := &mockAdminStore{feedback: []models.RetrievalFeedbackRecord{
		{ID: "1", Query: "Who is Big Jerry?", NormalizedQuery: "who is big jerry?", SearchMode: "hybrid", Outcome: models.RetrievalOutcomeHelpful, Signals: []string{models.RetrievalSignalConfirmedRecall}, CreatedAt: now},
		{ID: "2", Query: "Who is Big Jerry?", NormalizedQuery: "who is big jerry?", SearchMode: "hybrid", Outcome: models.RetrievalOutcomeMissed, Signals: []string{models.RetrievalSignalMissedKnownItem}, CreatedAt: now.Add(-time.Minute)},
		{ID: "3", Query: "policy stance", NormalizedQuery: "policy stance", SearchMode: "fulltext", Outcome: models.RetrievalOutcomeUnhelpful, Signals: []string{models.RetrievalSignalIrrelevantResult}, CreatedAt: now.Add(-2 * time.Minute)},
	}}
	svc := NewAdminService(store, nil, logrus.New())

	summary, err := svc.GetRetrievalFeedbackSummary(context.Background(), "tenant", models.RetrievalFeedbackListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("GetRetrievalFeedbackSummary: %v", err)
	}
	if summary.TotalEvents != 3 {
		t.Fatalf("TotalEvents = %d, want 3", summary.TotalEvents)
	}
	if summary.OutcomeCounts[models.RetrievalOutcomeHelpful] != 1 || summary.OutcomeCounts[models.RetrievalOutcomeMissed] != 1 {
		t.Fatalf("OutcomeCounts = %#v", summary.OutcomeCounts)
	}
	if summary.SignalCounts[models.RetrievalSignalConfirmedRecall] != 1 || summary.SignalCounts[models.RetrievalSignalMissedKnownItem] != 1 {
		t.Fatalf("SignalCounts = %#v", summary.SignalCounts)
	}
	if len(summary.QueryBreakdown) != 2 {
		t.Fatalf("len(QueryBreakdown) = %d, want 2", len(summary.QueryBreakdown))
	}
	if summary.QueryBreakdown[0].NormalizedQuery != "who is big jerry?" {
		t.Fatalf("first query breakdown = %#v", summary.QueryBreakdown[0])
	}
}
