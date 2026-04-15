package ingest

import "time"

// ProgressEvent reports ingest pipeline progress for long-running operations.
type ProgressEvent struct {
	Source        string
	Stage         string
	ChunkIndex    int
	TotalChunks   int
	Entities      int
	Relationships int
	Facts         int
	CreatedNodes  int
	UpdatedNodes  int
	SkippedNodes  int
	CreatedEdges  int
	SkippedEdges  int
	Errors        int
	Elapsed       time.Duration
	StageElapsed  time.Duration
}

// ProgressSink receives ingest progress events.
type ProgressSink func(ProgressEvent)

func emitProgress(sink ProgressSink, event ProgressEvent) {
	if sink != nil {
		sink(event)
	}
}
