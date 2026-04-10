package models_test

import (
	"strings"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

func ptr[T any](v T) *T { return &v }

func assertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}

	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected error containing %q, got %q", want, err.Error())
	}
}

func TestCreateNodeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     models.CreateNodeRequest
		wantErr string
	}{
		{name: "valid with id", req: models.CreateNodeRequest{ID: "n1", Type: "person", Label: "Alice"}},
		{name: "valid without id", req: models.CreateNodeRequest{Type: "person", Label: "Alice"}},
		{name: "missing type", req: models.CreateNodeRequest{Label: "Alice"}, wantErr: "type is required"},
		{name: "missing label", req: models.CreateNodeRequest{Type: "person"}, wantErr: "label is required"},
		{name: "label too long", req: models.CreateNodeRequest{Type: "p", Label: strings.Repeat("x", 10001)}, wantErr: "exceeds maximum length"},
		{name: "id too long", req: models.CreateNodeRequest{ID: strings.Repeat("x", 256), Type: "p", Label: "a"}, wantErr: "exceeds maximum length"},
		{name: "type too long", req: models.CreateNodeRequest{Type: strings.Repeat("x", 101), Label: "a"}, wantErr: "exceeds maximum length"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func TestCreateEdgeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     models.CreateEdgeRequest
		wantErr string
	}{
		{name: "valid", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "knows"}},
		{name: "missing source", req: models.CreateEdgeRequest{Target: "b", Relation: "knows"}, wantErr: "source is required"},
		{name: "missing target", req: models.CreateEdgeRequest{Source: "a", Relation: "knows"}, wantErr: "target is required"},
		{name: "missing relation", req: models.CreateEdgeRequest{Source: "a", Target: "b"}, wantErr: "relation is required"},
		{name: "weight too high", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", Weight: ptr(1001.0)}, wantErr: "weight must be between"},
		{name: "weight negative", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", Weight: ptr(-1.0)}, wantErr: "weight must be between"},
		{name: "source too long", req: models.CreateEdgeRequest{Source: strings.Repeat("x", 256), Target: "b", Relation: "r"}, wantErr: "exceeds maximum length"},
		{name: "valid date_start", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", DateStart: ptr("2020-01-01")}},
		{name: "valid date_start and date_end", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", DateStart: ptr("2020-01-01"), DateEnd: ptr("2025-12-31")}},
		{name: "invalid date_start", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", DateStart: ptr("not-a-date")}, wantErr: "date_start"},
		{name: "invalid date_end", req: models.CreateEdgeRequest{Source: "a", Target: "b", Relation: "r", DateEnd: ptr("bad")}, wantErr: "date_end"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func TestUpdateNodeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     models.UpdateNodeRequest
		wantErr string
	}{
		{name: "valid", req: models.UpdateNodeRequest{Label: ptr("new")}},
		{name: "empty type", req: models.UpdateNodeRequest{Type: ptr("")}, wantErr: "type cannot be empty"},
		{name: "empty label", req: models.UpdateNodeRequest{Label: ptr("")}, wantErr: "label cannot be empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func TestUpdateEdgeRequest_Validate(t *testing.T) {
	assertNoError(t, (&models.UpdateEdgeRequest{Weight: ptr(500.0)}).Validate())
	assertErrorContains(t, (&models.UpdateEdgeRequest{Weight: ptr(1001.0)}).Validate(), "weight must be between")
	assertNoError(t, (&models.UpdateEdgeRequest{DateStart: ptr("2020-01-01"), DateEnd: ptr("2025")}).Validate())
	assertErrorContains(t, (&models.UpdateEdgeRequest{DateStart: ptr("not-a-date")}).Validate(), "date_start")
	assertErrorContains(t, (&models.UpdateEdgeRequest{DateEnd: ptr("bad")}).Validate(), "date_end")
}

func TestSupersedeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     models.SupersedeRequest
		wantErr string
	}{
		{name: "valid", req: models.SupersedeRequest{OldID: "a", NewID: "b"}},
		{name: "missing old", req: models.SupersedeRequest{NewID: "b"}, wantErr: "old_id and new_id are required"},
		{name: "missing new", req: models.SupersedeRequest{OldID: "a"}, wantErr: "old_id and new_id are required"},
		{name: "same ids", req: models.SupersedeRequest{OldID: "a", NewID: "a"}, wantErr: "must be different"},
		{name: "old too long", req: models.SupersedeRequest{OldID: strings.Repeat("x", 256), NewID: "b"}, wantErr: "exceeds maximum length"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func TestCreateEpisodeRequest_Validate(t *testing.T) {
	startedAt := ptr(modelsTime("2026-04-10T12:00:00Z"))
	endedAt := ptr(modelsTime("2026-04-10T13:00:00Z"))

	tests := []struct {
		name    string
		req     models.CreateEpisodeRequest
		wantErr string
	}{
		{name: "valid", req: models.CreateEpisodeRequest{Title: "Kickoff", StartedAt: startedAt, EndedAt: endedAt}},
		{name: "missing title", req: models.CreateEpisodeRequest{}, wantErr: "title: label is required"},
		{name: "bad status", req: models.CreateEpisodeRequest{Title: "Kickoff", Status: "stale"}, wantErr: "status must be one of"},
		{name: "bad time range", req: models.CreateEpisodeRequest{Title: "Kickoff", StartedAt: endedAt, EndedAt: startedAt}, wantErr: "ended_at must be on or after started_at"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func TestCreateEventRecordRequest_Validate(t *testing.T) {
	confidence := 0.75
	startedAt := ptr(modelsTime("2026-04-10T12:00:00Z"))
	endedAt := ptr(modelsTime("2026-04-10T13:00:00Z"))

	tests := []struct {
		name    string
		req     models.CreateEventRecordRequest
		wantErr string
	}{
		{name: "valid", req: models.CreateEventRecordRequest{Kind: models.EventKindDecision, Title: "Do it", Confidence: &confidence, Links: []models.CreateEventLinkRequest{{NodeID: "n1", Role: "owner"}}}},
		{name: "missing kind", req: models.CreateEventRecordRequest{Title: "No kind"}, wantErr: "kind must be one of"},
		{name: "missing title", req: models.CreateEventRecordRequest{Kind: models.EventKindTask}, wantErr: "title: label is required"},
		{name: "bad confidence", req: models.CreateEventRecordRequest{Kind: models.EventKindTask, Title: "Oops", Confidence: ptr(1.5)}, wantErr: "confidence must be between 0 and 1"},
		{name: "bad time range", req: models.CreateEventRecordRequest{Kind: models.EventKindTask, Title: "Oops", OccurredStartAt: endedAt, OccurredEndAt: startedAt}, wantErr: "occurred_end_at must be on or after occurred_start_at"},
		{name: "missing link role", req: models.CreateEventRecordRequest{Kind: models.EventKindTask, Title: "Oops", Links: []models.CreateEventLinkRequest{{NodeID: "n1"}}}, wantErr: "relation is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr != "" {
				assertErrorContains(t, err, tc.wantErr)
				return
			}
			assertNoError(t, err)
		})
	}
}

func modelsTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		panic(err)
	}
	return t
}
