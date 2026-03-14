package ingest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/ingest"
)

var errTest = errors.New("test LLM error")

func TestIngest_DryRun(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Alice", "type": "person", "properties": {}, "description": "A dev"}
			],
			"relationships": [
				{"source": "Alice", "target": "Go", "relation": "uses", "confidence": 0.9}
			],
			"facts": []
		}`,
	}

	gc := newMockGraphClient()
	ext := ingest.NewExtractor(llm)
	w := ingest.NewWriter(gc, "test")
	ing := ingest.NewIngester(ext, w, nil)

	report, err := ing.Ingest(context.Background(), strings.NewReader("Alice uses Go."), ingest.IngestOpts{
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Chunks != 1 {
		t.Errorf("expected 1 chunk, got %d", report.Chunks)
	}

	if report.CreatedNodes != 1 {
		t.Errorf("expected 1 created node in dry run, got %d", report.CreatedNodes)
	}

	// Dry run should not create any actual nodes.
	if len(gc.createdNodes) != 0 {
		t.Errorf("expected 0 actual nodes created in dry run, got %d", len(gc.createdNodes))
	}
}

func TestIngest_FullPipeline(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Alice", "type": "person", "properties": {"role": "dev"}, "description": "A dev"},
				{"name": "Persistor", "type": "project", "properties": {}, "description": "A KG service"}
			],
			"relationships": [
				{"source": "Alice", "target": "Persistor", "relation": "works_on", "confidence": 0.95}
			],
			"facts": [
				{"subject": "Alice", "property": "language", "value": "Go"}
			]
		}`,
	}

	gc := newMockGraphClient()
	ext := ingest.NewExtractor(llm)
	w := ingest.NewWriter(gc, "test")
	ing := ingest.NewIngester(ext, w, nil)

	report, err := ing.Ingest(context.Background(), strings.NewReader("Alice works on Persistor."), ingest.IngestOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.CreatedNodes != 2 {
		t.Errorf("expected 2 created nodes, got %d", report.CreatedNodes)
	}

	if report.CreatedEdges != 1 {
		t.Errorf("expected 1 created edge, got %d", report.CreatedEdges)
	}

	if len(gc.createdEdges) != 1 {
		t.Errorf("expected 1 edge created, got %d", len(gc.createdEdges))
	}
}

func TestIngest_LLMError(t *testing.T) {
	llm := &mockLLM{
		err: errTest,
	}

	gc := newMockGraphClient()
	ext := ingest.NewExtractor(llm)
	w := ingest.NewWriter(gc, "test")
	ing := ingest.NewIngester(ext, w, nil)

	report, err := ing.Ingest(context.Background(), strings.NewReader("some text"), ingest.IngestOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Errors) != 1 {
		t.Errorf("expected 1 error in report, got %d", len(report.Errors))
	}
}

func TestIngest_EmptyInput(t *testing.T) {
	llm := &mockLLM{response: `{"entities":[],"relationships":[],"facts":[]}`}

	gc := newMockGraphClient()
	ext := ingest.NewExtractor(llm)
	w := ingest.NewWriter(gc, "test")
	ing := ingest.NewIngester(ext, w, nil)

	report, err := ing.Ingest(context.Background(), strings.NewReader(""), ingest.IngestOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Chunks != 0 {
		t.Errorf("expected 0 chunks, got %d", report.Chunks)
	}
}
