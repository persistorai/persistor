// Package services provides business logic for the persistor.
package service

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/metrics"
)

// EmbedJob represents a request to generate and store an embedding for a node.
type EmbedJob struct {
	TenantID string
	NodeID   string
	Text     string // "type:label"
}

// EmbeddingUpdater stores a generated embedding for a node.
type EmbeddingUpdater interface {
	UpdateNodeEmbedding(ctx context.Context, tenantID, nodeID string, embedding []float32) error
}

// EmbedWorker processes embedding jobs asynchronously with retry.
type EmbedWorker struct {
	embed       *EmbeddingService
	repo        EmbeddingUpdater
	log         *logrus.Logger
	jobs        chan EmbedJob
	maxJobs     int
	concurrency int
}

// NewEmbedWorker creates a worker with the given queue capacity and concurrency.
func NewEmbedWorker(embed *EmbeddingService, repo EmbeddingUpdater, log *logrus.Logger, queueSize, concurrency int) *EmbedWorker {
	if queueSize <= 0 {
		queueSize = 1000
	}
	if concurrency <= 0 {
		concurrency = 4
	}

	return &EmbedWorker{
		embed:       embed,
		repo:        repo,
		log:         log,
		jobs:        make(chan EmbedJob, queueSize),
		maxJobs:     queueSize,
		concurrency: concurrency,
	}
}

// Enqueue adds an embedding job. Non-blocking; drops the job if the queue is full.
func (w *EmbedWorker) Enqueue(job EmbedJob) {
	select {
	case w.jobs <- job:
		metrics.EmbedQueueDepth.Set(float64(len(w.jobs)))
	default:
		w.log.WithField("node_id", job.NodeID).Warn("embedding queue full, dropping job")
	}
}

// Run spawns N worker goroutines and blocks until the context is cancelled
// and all workers have drained. Call in a goroutine.
func (w *EmbedWorker) Run(ctx context.Context) {
	var wg sync.WaitGroup

	w.log.WithField("concurrency", w.concurrency).Info("starting embed workers")

	for i := range w.concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w.runWorker(ctx, id)
		}(i)
	}

	wg.Wait()
	w.log.Info("all embed workers stopped")
}

func (w *EmbedWorker) runWorker(ctx context.Context, id int) {
	w.log.WithField("worker_id", id).Debug("embed worker started")
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs:
			metrics.EmbedQueueDepth.Set(float64(len(w.jobs)))
			w.processWithRetry(ctx, job)
		}
	}
}

const (
	maxRetries     = 3
	baseRetryDelay = 2 * time.Second
)

func (w *EmbedWorker) processWithRetry(ctx context.Context, job EmbedJob) {
	for attempt := range maxRetries {
		if ctx.Err() != nil {
			return
		}

		embedding, err := w.embed.Generate(ctx, job.Text)
		if err != nil {
			w.log.WithError(err).WithFields(logrus.Fields{
				"node_id": job.NodeID,
				"attempt": attempt + 1,
			}).Warn("embedding generation failed")

			if attempt < maxRetries-1 {
				delay := baseRetryDelay * (1 << attempt) // exponential backoff
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}

			continue
		}

		if err := w.repo.UpdateNodeEmbedding(ctx, job.TenantID, job.NodeID, embedding); err != nil {
			w.log.WithError(err).WithField("node_id", job.NodeID).Error("storing embedding")
		} else {
			w.log.WithField("node_id", job.NodeID).Debug("embedding stored")
		}

		return
	}

	w.log.WithField("node_id", job.NodeID).Error("embedding failed after all retries")
}
