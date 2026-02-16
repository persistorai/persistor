package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func newTestNodeService(store *mockNodeStore, embedEnq *mockEmbedEnqueuer, auditor *mockAuditor) (*NodeService, *AuditWorker) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	aw := NewAuditWorker(auditor, log, 100)
	ctx, cancel := context.WithCancel(context.Background())
	go aw.Run(ctx)

	if embedEnq == nil {
		embedEnq = &mockEmbedEnqueuer{}
	}

	svc := NewNodeService(store, embedEnq, aw, log)

	// Return cleanup via t.Cleanup in caller; we return aw for stop.
	_ = cancel // caller should defer cancel()
	return svc, aw
}

func TestNodeService_CreateNode(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantErr   bool
		wantAudit bool
	}{
		{name: "success", storeErr: nil, wantErr: false, wantAudit: true},
		{name: "store error", storeErr: errors.New("db down"), wantErr: true, wantAudit: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockNodeStore{
				createNode: func(_ context.Context, _ string, _ models.CreateNodeRequest) (*models.Node, error) {
					if tc.storeErr != nil {
						return nil, tc.storeErr
					}
					return &models.Node{ID: "n1", Type: "concept", Label: "Test"}, nil
				},
			}
			auditor := &mockAuditor{}
			embedEnq := &mockEmbedEnqueuer{}
			log := logrus.New()
			log.SetLevel(logrus.ErrorLevel)

			aw := NewAuditWorker(auditor, log, 100)
			ctx, cancel := context.WithCancel(context.Background())
			go aw.Run(ctx)
			defer cancel()

			svc := NewNodeService(store, embedEnq, aw, log)

			node, err := svc.CreateNode(context.Background(), "tenant1", models.CreateNodeRequest{
				Type: "concept", Label: "Test",
			})

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if node.ID != "n1" {
				t.Errorf("got node ID %q, want %q", node.ID, "n1")
			}
			if len(store.calls) != 1 || store.calls[0] != "CreateNode" {
				t.Errorf("expected CreateNode call, got %v", store.calls)
			}

			// Check embed enqueue
			if len(embedEnq.jobs) != 1 {
				t.Errorf("expected 1 embed job, got %d", len(embedEnq.jobs))
			}

			// Wait for async audit
			time.Sleep(50 * time.Millisecond)
			if tc.wantAudit {
				calls := auditor.getCalls()
				if len(calls) != 1 {
					t.Errorf("expected 1 audit call, got %d", len(calls))
				} else if calls[0].Action != "node.create" {
					t.Errorf("audit action = %q, want %q", calls[0].Action, "node.create")
				}
			}
		})
	}
}

func TestNodeService_UpdateNode(t *testing.T) {
	store := &mockNodeStore{
		updateNode: func(_ context.Context, _, _ string, _ models.UpdateNodeRequest) (*models.Node, error) {
			return &models.Node{ID: "n1", Type: "person", Label: "Updated"}, nil
		},
	}
	auditor := &mockAuditor{}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	aw := NewAuditWorker(auditor, log, 100)
	ctx, cancel := context.WithCancel(context.Background())
	go aw.Run(ctx)
	defer cancel()

	svc := NewNodeService(store, &mockEmbedEnqueuer{}, aw, log)

	lbl := "Updated"
	node, err := svc.UpdateNode(context.Background(), "t1", "n1", models.UpdateNodeRequest{Label: &lbl})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Label != "Updated" {
		t.Errorf("label = %q, want %q", node.Label, "Updated")
	}
	if len(store.calls) != 1 || store.calls[0] != "UpdateNode" {
		t.Errorf("expected UpdateNode call, got %v", store.calls)
	}

	time.Sleep(50 * time.Millisecond)
	calls := auditor.getCalls()
	if len(calls) != 1 || calls[0].Action != "node.update" {
		t.Errorf("expected node.update audit, got %v", calls)
	}
}

func TestNodeService_DeleteNode(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantAudit bool
	}{
		{name: "success", storeErr: nil, wantAudit: true},
		{name: "store error", storeErr: errors.New("not found"), wantAudit: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockNodeStore{
				deleteNode: func(_ context.Context, _, _ string) error { return tc.storeErr },
			}
			auditor := &mockAuditor{}
			log := logrus.New()
			log.SetLevel(logrus.ErrorLevel)
			aw := NewAuditWorker(auditor, log, 100)
			ctx, cancel := context.WithCancel(context.Background())
			go aw.Run(ctx)
			defer cancel()

			svc := NewNodeService(store, &mockEmbedEnqueuer{}, aw, log)
			err := svc.DeleteNode(context.Background(), "t1", "n1")

			if tc.storeErr != nil && err == nil {
				t.Fatal("expected error")
			}

			time.Sleep(50 * time.Millisecond)
			calls := auditor.getCalls()
			if tc.wantAudit && (len(calls) == 0 || calls[0].Action != "node.delete") {
				t.Errorf("expected node.delete audit, got %v", calls)
			}
			if !tc.wantAudit && len(calls) > 0 {
				t.Errorf("expected no audit, got %v", calls)
			}
		})
	}
}

func TestNodeService_GetNode(t *testing.T) {
	store := &mockNodeStore{
		getNode: func(_ context.Context, _, _ string) (*models.Node, error) {
			return &models.Node{ID: "n1", Label: "Hello"}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewNodeService(store, &mockEmbedEnqueuer{}, nil, log)

	node, err := svc.GetNode(context.Background(), "t1", "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.ID != "n1" {
		t.Errorf("got %q, want %q", node.ID, "n1")
	}
	if len(store.calls) != 1 || store.calls[0] != "GetNode" {
		t.Errorf("expected GetNode, got %v", store.calls)
	}
}

func TestNodeService_ListNodes(t *testing.T) {
	store := &mockNodeStore{
		listNodes: func(_ context.Context, _ string, _ string, _ float64, _, _ int) ([]models.Node, bool, error) {
			return []models.Node{{ID: "n1"}, {ID: "n2"}}, true, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewNodeService(store, &mockEmbedEnqueuer{}, nil, log)

	nodes, hasMore, err := svc.ListNodes(context.Background(), "t1", "", 0, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("got %d nodes, want 2", len(nodes))
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
}
