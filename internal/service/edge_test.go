package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func testLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	return log
}

func TestEdgeService_CreateEdge(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantErr   bool
		wantAudit bool
	}{
		{name: "success", wantAudit: true},
		{name: "store error", storeErr: errors.New("fail"), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockEdgeStore{
				createEdge: func(_ context.Context, _ string, _ models.CreateEdgeRequest) (*models.Edge, error) {
					if tc.storeErr != nil {
						return nil, tc.storeErr
					}
					return &models.Edge{Source: "a", Target: "b", Relation: "knows"}, nil
				},
			}
			auditor := &mockAuditor{}
			log := testLogger()
			aw := NewAuditWorker(auditor, log, 100)
			ctx, cancel := context.WithCancel(context.Background())
			go aw.Run(ctx)
			defer cancel()

			svc := NewEdgeService(store, aw, log)
			edge, err := svc.CreateEdge(context.Background(), "t1", models.CreateEdgeRequest{
				Source: "a", Target: "b", Relation: "knows",
			})

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if edge.Source != "a" {
				t.Errorf("source = %q, want %q", edge.Source, "a")
			}

			time.Sleep(50 * time.Millisecond)
			if tc.wantAudit {
				calls := auditor.getCalls()
				if len(calls) != 1 || calls[0].Action != "edge.create" {
					t.Errorf("expected edge.create audit, got %v", calls)
				}
			}
		})
	}
}

func TestEdgeService_UpdateEdge(t *testing.T) {
	w := 0.5
	store := &mockEdgeStore{
		updateEdge: func(_ context.Context, _, _, _, _ string, _ models.UpdateEdgeRequest) (*models.Edge, error) {
			return &models.Edge{Source: "a", Target: "b", Relation: "knows", Weight: 0.5}, nil
		},
	}
	auditor := &mockAuditor{}
	log := testLogger()
	aw := NewAuditWorker(auditor, log, 100)
	ctx, cancel := context.WithCancel(context.Background())
	go aw.Run(ctx)
	defer cancel()

	svc := NewEdgeService(store, aw, log)
	edge, err := svc.UpdateEdge(context.Background(), "t1", "a", "b", "knows", models.UpdateEdgeRequest{Weight: &w})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edge.Weight != 0.5 {
		t.Errorf("weight = %f, want 0.5", edge.Weight)
	}

	time.Sleep(50 * time.Millisecond)
	calls := auditor.getCalls()
	if len(calls) != 1 || calls[0].Action != "edge.update" {
		t.Errorf("expected edge.update audit, got %v", calls)
	}
}

func TestEdgeService_DeleteEdge(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantAudit bool
	}{
		{name: "success", wantAudit: true},
		{name: "store error", storeErr: errors.New("fail")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockEdgeStore{
				deleteEdge: func(_ context.Context, _, _, _, _ string) error { return tc.storeErr },
			}
			auditor := &mockAuditor{}
			log := testLogger()
			aw := NewAuditWorker(auditor, log, 100)
			ctx, cancel := context.WithCancel(context.Background())
			go aw.Run(ctx)
			defer cancel()

			svc := NewEdgeService(store, aw, log)
			err := svc.DeleteEdge(context.Background(), "t1", "a", "b", "knows")

			if tc.storeErr != nil && err == nil {
				t.Fatal("expected error")
			}

			time.Sleep(50 * time.Millisecond)
			calls := auditor.getCalls()
			if tc.wantAudit && (len(calls) == 0 || calls[0].Action != "edge.delete") {
				t.Errorf("expected edge.delete audit, got %v", calls)
			}
			if !tc.wantAudit && len(calls) > 0 {
				t.Errorf("expected no audit, got %v", calls)
			}
		})
	}
}

func TestEdgeService_ListEdges(t *testing.T) {
	store := &mockEdgeStore{
		listEdges: func(_ context.Context, _, _, _, _ string, _, _ int) ([]models.Edge, bool, error) {
			return []models.Edge{{Source: "a", Target: "b", Relation: "knows"}}, false, nil
		},
	}
	svc := NewEdgeService(store, nil, testLogger())

	edges, hasMore, err := svc.ListEdges(context.Background(), "t1", "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 1 {
		t.Errorf("got %d edges, want 1", len(edges))
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
}
