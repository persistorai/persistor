package service

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestAuditWorker_ProcessesJob(t *testing.T) {
	auditor := &mockAuditor{}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	aw := NewAuditWorker(auditor, log, 10)
	ctx, cancel := context.WithCancel(context.Background())
	go aw.Run(ctx)

	aw.Enqueue(&AuditJob{
		TenantID:   "t1",
		Action:     "node.create",
		EntityType: "node",
		EntityID:   "n1",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	calls := auditor.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(calls))
	}
	if calls[0].Action != "node.create" {
		t.Errorf("action = %q, want %q", calls[0].Action, "node.create")
	}
	if calls[0].EntityID != "n1" {
		t.Errorf("entity_id = %q, want %q", calls[0].EntityID, "n1")
	}
}

func TestAuditWorker_DropsWhenFull(t *testing.T) {
	auditor := &mockAuditor{}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Queue size 2, don't start the worker so it can't drain.
	aw := NewAuditWorker(auditor, log, 2)

	// Fill the queue.
	aw.Enqueue(&AuditJob{Action: "a"})
	aw.Enqueue(&AuditJob{Action: "b"})

	// This should be dropped (non-blocking).
	done := make(chan struct{})
	go func() {
		aw.Enqueue(&AuditJob{Action: "c"})
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block.
	case <-time.After(time.Second):
		t.Fatal("Enqueue blocked when queue was full")
	}

	// Only 2 in queue.
	if len(aw.jobs) != 2 {
		t.Errorf("queue len = %d, want 2", len(aw.jobs))
	}
}

func TestAuditWorker_StopDrains(t *testing.T) {
	auditor := &mockAuditor{}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	aw := NewAuditWorker(auditor, log, 100)

	// Enqueue before starting.
	for i := range 5 {
		aw.Enqueue(&AuditJob{Action: "drain", EntityID: string(rune('a' + i))})
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		aw.Run(ctx)
		close(done)
	}()

	// Let worker start and process, then cancel to trigger drain.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run didn't return after cancel")
	}

	calls := auditor.getCalls()
	if len(calls) != 5 {
		t.Errorf("expected 5 drained audit calls, got %d", len(calls))
	}
}
