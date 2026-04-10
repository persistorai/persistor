package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/persistorai/persistor/internal/api"
	"github.com/persistorai/persistor/internal/models"
)

func TestRecordRetrievalFeedback(t *testing.T) {
	repo := &mockAdminRepo{recordFeedbackFn: func(_ context.Context, _ string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
		return &models.RetrievalFeedbackRecord{ID: "fb-1", Query: req.Query, Outcome: req.Outcome}, nil
	}}
	r := newTestRouter()
	h := api.NewAdminHandler(repo, nil, testLogger())
	r.POST("/admin/retrieval-feedback", h.RecordRetrievalFeedback)

	w := doRequest(r, http.MethodPost, "/admin/retrieval-feedback", `{"query":"Who is Big Jerry?","outcome":"helpful"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetRetrievalFeedbackSummary(t *testing.T) {
	repo := &mockAdminRepo{summaryFn: func(_ context.Context, _ string, opts models.RetrievalFeedbackListOpts) (*models.RetrievalFeedbackSummary, error) {
		if opts.Limit != 5 {
			t.Fatalf("limit = %d, want 5", opts.Limit)
		}
		return &models.RetrievalFeedbackSummary{TotalEvents: 1, OutcomeCounts: map[string]int{models.RetrievalOutcomeHelpful: 1}}, nil
	}}
	r := newTestRouter()
	h := api.NewAdminHandler(repo, nil, testLogger())
	r.GET("/admin/retrieval-feedback", h.GetRetrievalFeedbackSummary)

	w := doRequest(r, http.MethodGet, "/admin/retrieval-feedback?limit=5", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body models.RetrievalFeedbackSummary
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.TotalEvents != 1 {
		t.Fatalf("TotalEvents = %d, want 1", body.TotalEvents)
	}
}
