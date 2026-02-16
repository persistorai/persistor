package service

import (
	"context"

	"github.com/sirupsen/logrus"
)

// AuditJob represents a single audit entry to be recorded.
type AuditJob struct {
	TenantID   string
	Action     string
	EntityType string
	EntityID   string
	Actor      string
	Detail     map[string]any
}

// AuditWorker buffers audit entries and writes them via a single worker goroutine.
type AuditWorker struct {
	auditor Auditor
	log     *logrus.Logger
	jobs    chan *AuditJob
}

// NewAuditWorker creates an AuditWorker with the given queue capacity.
func NewAuditWorker(auditor Auditor, log *logrus.Logger, queueSize int) *AuditWorker {
	if queueSize <= 0 {
		queueSize = 1000
	}
	return &AuditWorker{
		auditor: auditor,
		log:     log,
		jobs:    make(chan *AuditJob, queueSize),
	}
}

// Enqueue adds an audit job. Non-blocking; drops the job if the queue is full.
func (w *AuditWorker) Enqueue(job *AuditJob) {
	select {
	case w.jobs <- job:
	default:
		w.log.WithField("action", job.Action).Warn("audit queue full, dropping entry")
	}
}

// Run processes audit jobs until the context is cancelled, then drains remaining jobs.
func (w *AuditWorker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.drain()
			return
		case job := <-w.jobs:
			w.process(job)
		}
	}
}

func (w *AuditWorker) drain() {
	for {
		select {
		case job := <-w.jobs:
			w.process(job)
		default:
			return
		}
	}
}

func (w *AuditWorker) process(job *AuditJob) {
	if err := w.auditor.RecordAudit(
		context.Background(), job.TenantID, job.Action, job.EntityType, job.EntityID, job.Actor, job.Detail,
	); err != nil {
		w.log.WithError(err).Warn("audit record failed")
	}
}
