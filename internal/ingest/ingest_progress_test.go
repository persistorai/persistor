package ingest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/ingest"
)

func TestIngest_ReportsProgressAndMetrics(t *testing.T) {
	llm := &mockLLM{response: `{
		"entities": [
			{"name": "Alice", "type": "person", "properties": {}, "description": "A dev"},
			{"name": "Persistor", "type": "project", "properties": {}, "description": "A project"}
		],
		"relationships": [{"source": "Alice", "target": "Persistor", "relation": "works_on", "confidence": 0.9}],
		"facts": [{"subject": "Alice", "property": "language", "value": "Go"}]
	}`}

	gc := newMockGraphClient()
	ext := ingest.NewExtractor(llm)
	w := ingest.NewWriter(gc, "test")
	ing := ingest.NewIngester(ext, w, nil)

	var events []ingest.ProgressEvent
	report, err := ing.Ingest(context.Background(), strings.NewReader("Alice uses Go."), ingest.IngestOpts{
		Source:   "test",
		Progress: func(event ingest.ProgressEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	if events[0].Stage != "chunked" {
		t.Fatalf("expected first event to be chunked, got %q", events[0].Stage)
	}
	if events[len(events)-1].Stage != "done" {
		t.Fatalf("expected last event to be done, got %q", events[len(events)-1].Stage)
	}
	if report.ExtractedEntities == 0 {
		t.Fatalf("expected extracted entities to be tracked, got %+v", report)
	}
	if report.ExtractedFacts != 1 {
		t.Fatalf("expected extracted facts to be tracked, got %+v", report)
	}
	if report.TotalDuration <= 0 {
		t.Fatalf("expected total duration to be recorded, got %s", report.TotalDuration)
	}
}
