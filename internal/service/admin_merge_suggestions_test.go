package service

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

type mockAdminStore struct {
	pairs       []store.DuplicateCandidatePair
	reprocess   []store.ReprocessableNode
	maintenance []store.ReprocessableNode
	feedback    []models.RetrievalFeedbackRecord
}

func (m *mockAdminStore) ListNodesWithoutEmbeddings(_ context.Context, _ string, _ int) ([]models.NodeSummary, error) {
	return nil, nil
}

func (m *mockAdminStore) ListNodesForReprocess(_ context.Context, _ string, _ int) ([]store.ReprocessableNode, error) {
	return m.reprocess, nil
}

func (m *mockAdminStore) ListNodesForMaintenance(_ context.Context, _ string, _ int) ([]store.ReprocessableNode, error) {
	return m.maintenance, nil
}

func (m *mockAdminStore) CountNodesForReprocess(_ context.Context, _ string) (int, int, int, error) {
	return 0, 0, 0, nil
}

func (m *mockAdminStore) UpdateNodeSearchText(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockAdminStore) ListDuplicateCandidatePairs(_ context.Context, _, _ string, _ int) ([]store.DuplicateCandidatePair, error) {
	return m.pairs, nil
}

func (m *mockAdminStore) CreateRetrievalFeedback(_ context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
	record := &models.RetrievalFeedbackRecord{
		ID:               "feedback-1",
		TenantID:         tenantID,
		Query:            req.Query,
		NormalizedQuery:  models.NormalizeRetrievalQuery(req.Query),
		SearchMode:       req.SearchMode,
		Intent:           req.Intent,
		InternalRerank:   req.InternalRerank,
		RerankProfile:    req.RerankProfile,
		Outcome:          req.Outcome,
		Signals:          models.DeriveRetrievalSignals(req),
		RetrievedNodeIDs: req.RetrievedNodeIDs,
		SelectedNodeIDs:  req.SelectedNodeIDs,
		ExpectedNodeIDs:  req.ExpectedNodeIDs,
		Note:             req.Note,
	}
	m.feedback = append([]models.RetrievalFeedbackRecord{*record}, m.feedback...)
	return record, nil
}

func (m *mockAdminStore) ListRetrievalFeedback(_ context.Context, _ string, _ models.RetrievalFeedbackListOpts) ([]models.RetrievalFeedbackRecord, error) {
	return append([]models.RetrievalFeedbackRecord(nil), m.feedback...), nil
}

func TestListMergeSuggestions(t *testing.T) {
	svc := NewAdminService(&mockAdminStore{pairs: []store.DuplicateCandidatePair{
		{
			Left:        models.Node{ID: "node-a", Type: "person", Label: "Bill Gates", Salience: 0.9, Properties: map[string]any{"email": "bill@example.com"}},
			Right:       models.Node{ID: "node-b", Type: "person", Label: "bill gates", Salience: 0.4, Properties: map[string]any{"email": "bill@example.com"}},
			SharedNames: []string{"bill gates"},
			SameLabel:   true,
		},
		{
			Left:              models.Node{ID: "node-c", Type: "company", Label: "Acme", Salience: 0.5, Properties: map[string]any{"domain": "acme.test"}},
			Right:             models.Node{ID: "node-d", Type: "company", Label: "Acme Corp", Salience: 0.6, Properties: map[string]any{"domain": "acme.test"}},
			SharedNames:       []string{"acme", "acme corp"},
			LabelAliasOverlap: true,
		},
	}}, nil, logrus.New())

	suggestions, err := svc.ListMergeSuggestions(context.Background(), "tenant", models.MergeSuggestionListOpts{Limit: 10, MinScore: 0.6})
	if err != nil {
		t.Fatalf("ListMergeSuggestions: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("len(suggestions) = %d, want 2", len(suggestions))
	}
	if suggestions[0].Canonical.ID != "node-a" {
		t.Fatalf("canonical = %q, want node-a", suggestions[0].Canonical.ID)
	}
	if suggestions[0].Score < 0.9 {
		t.Fatalf("score = %v, want >= 0.9", suggestions[0].Score)
	}
	if len(suggestions[0].Reasons) < 2 {
		t.Fatalf("reasons = %#v, want multiple reasons", suggestions[0].Reasons)
	}
}

func TestListMergeSuggestionsFiltersBelowThreshold(t *testing.T) {
	svc := NewAdminService(&mockAdminStore{pairs: []store.DuplicateCandidatePair{{
		Left:        models.Node{ID: "node-a", Type: "person", Label: "Alice", Salience: 0.5, Properties: map[string]any{}},
		Right:       models.Node{ID: "node-b", Type: "person", Label: "Alicia", Salience: 0.4, Properties: map[string]any{}},
		SharedNames: []string{"ali"},
	}}}, nil, logrus.New())

	suggestions, err := svc.ListMergeSuggestions(context.Background(), "tenant", models.MergeSuggestionListOpts{MinScore: 0.6})
	if err != nil {
		t.Fatalf("ListMergeSuggestions: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("len(suggestions) = %d, want 0", len(suggestions))
	}
}

func TestRunMaintenance(t *testing.T) {
	embed := &mockEmbedEnqueuer{}
	svc := NewAdminService(&mockAdminStore{
		maintenance: []store.ReprocessableNode{
			{ID: "node-a", Type: "person", Label: "Alice", Properties: map[string]any{"summary": "keeps notes", models.FactEvidenceProperty: map[string]any{"summary": []map[string]any{{"supersedes_prior": true}}}}, CurrentSearchText: "", NeedsEmbedding: true, HasFactEvidence: true, HasSupersededFacts: true},
			{ID: "node-b", Type: "project", Label: "Persistor", Properties: map[string]any{"status": "active"}, CurrentSearchText: models.BuildNodeSearchText(&models.Node{Type: "project", Label: "Persistor", Properties: map[string]any{"status": "active"}}), NodeSuperseded: true},
		},
		pairs: []store.DuplicateCandidatePair{{Left: models.Node{ID: "node-a"}, Right: models.Node{ID: "node-c"}}},
	}, embed, logrus.New())

	result, err := svc.RunMaintenance(context.Background(), "tenant", models.MaintenanceRunRequest{RefreshSearchText: true, RefreshEmbeddings: true, ScanStaleFacts: true, IncludeDuplicateCandidates: true})
	if err != nil {
		t.Fatalf("RunMaintenance: %v", err)
	}
	if result.Scanned != 2 || result.UpdatedSearchText != 1 || result.QueuedEmbeddings != 1 {
		t.Fatalf("unexpected maintenance result: %+v", result)
	}
	if result.StaleFactNodes != 1 || result.SupersededNodes != 2 || result.DuplicateCandidatePairs != 1 {
		t.Fatalf("unexpected maintenance scan counts: %+v", result)
	}
	if len(embed.jobs) != 1 || embed.jobs[0].NodeID != "node-a" {
		t.Fatalf("embed jobs = %#v, want one job for node-a", embed.jobs)
	}
}
