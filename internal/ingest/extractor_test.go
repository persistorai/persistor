package ingest_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/ingest"
)

// mockLLM implements ingest.LLMClient for testing.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Chat(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func TestExtract_ValidJSON(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Alice", "type": "person", "properties": {}, "description": "A developer"}
			],
			"relationships": [
				{"source": "Alice", "target": "Persistor", "relation": "works_on", "confidence": 0.9}
			],
			"facts": [
				{"subject": "Alice", "property": "role", "value": "engineer"}
			]
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Alice works on Persistor.")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	if result.Entities[0].Name != "Alice" {
		t.Errorf("expected entity name Alice, got %q", result.Entities[0].Name)
	}

	if len(result.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(result.Relationships))
	}

	if len(result.Facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(result.Facts))
	}
}

func TestExtract_TruncatedJSON(t *testing.T) {
	llm := &mockLLM{
		response: `{"entities": [{"name": "Bob", "type": "person", "properties": {}, "description": "A manager"}], "relationships": [`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Bob is a manager.")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	if result.Entities[0].Name != "Bob" {
		t.Errorf("expected entity name Bob, got %q", result.Entities[0].Name)
	}
}

func TestExtract_FencedJSON(t *testing.T) {
	llm := &mockLLM{
		response: "```json\n{\"entities\": [{\"name\": \"Eve\", \"type\": \"person\", \"properties\": {}, \"description\": \"A researcher\"}], \"relationships\": [], \"facts\": []}\n```",
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Eve is a researcher.")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	if result.Entities[0].Name != "Eve" {
		t.Errorf("expected entity name Eve, got %q", result.Entities[0].Name)
	}
}

func TestExtract_InvalidEntityTypesFiltered(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Alice", "type": "person", "properties": {}, "description": "Valid"},
				{"name": "FooBar", "type": "widget", "properties": {}, "description": "Invalid type"}
			],
			"relationships": [],
			"facts": []
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity after filtering, got %d", len(result.Entities))
	}

	if result.Entities[0].Name != "Alice" {
		t.Errorf("expected Alice to remain, got %q", result.Entities[0].Name)
	}
}

func TestExtract_EmptyResponse(t *testing.T) {
	llm := &mockLLM{response: ""}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(result.Entities))
	}
}

func TestExtract_WhitespaceResponse(t *testing.T) {
	llm := &mockLLM{response: "   \n  "}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(result.Entities))
	}
}

func TestExtract_EntityTypeCaseInsensitive(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Google", "type": "Company", "properties": {}, "description": "A tech company"}
			],
			"relationships": [],
			"facts": []
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	if result.Entities[0].Type != "company" {
		t.Errorf("expected normalized type 'company', got %q", result.Entities[0].Type)
	}
}

func TestExtract_TemporalRelationshipFields(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Alice", "type": "person", "properties": {}, "description": "A developer"}
			],
			"relationships": [
				{"source": "Alice", "target": "Acme", "relation": "worked_at", "confidence": 0.95, "date_start": "2009", "date_end": "2022", "is_current": false}
			],
			"facts": []
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Alice worked at Acme from 2009 to 2022.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(result.Relationships))
	}

	rel := result.Relationships[0]

	if rel.DateStart == nil || *rel.DateStart != "2009" {
		t.Errorf("expected date_start '2009', got %v", rel.DateStart)
	}

	if rel.DateEnd == nil || *rel.DateEnd != "2022" {
		t.Errorf("expected date_end '2022', got %v", rel.DateEnd)
	}

	if rel.IsCurrent == nil {
		t.Fatal("expected is_current to be set")
	}

	if *rel.IsCurrent {
		t.Error("expected is_current false")
	}
}

func TestExtract_CurrentRelationshipFields(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Bob", "type": "person", "properties": {}, "description": "An engineer"}
			],
			"relationships": [
				{"source": "Bob", "target": "Widgets Inc", "relation": "works_at", "confidence": 0.9, "date_start": "2020", "is_current": true}
			],
			"facts": []
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Bob currently works at Widgets Inc since 2020.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(result.Relationships))
	}

	rel := result.Relationships[0]

	if rel.DateStart == nil || *rel.DateStart != "2020" {
		t.Errorf("expected date_start '2020', got %v", rel.DateStart)
	}

	if rel.DateEnd != nil {
		t.Errorf("expected date_end nil for current relationship, got %v", rel.DateEnd)
	}

	if rel.IsCurrent == nil || !*rel.IsCurrent {
		t.Error("expected is_current true for current relationship")
	}
}

func TestExtract_NoTemporalFields(t *testing.T) {
	llm := &mockLLM{
		response: `{
			"entities": [
				{"name": "Carol", "type": "person", "properties": {}, "description": "A designer"}
			],
			"relationships": [
				{"source": "Carol", "target": "DesignCo", "relation": "works_at", "confidence": 0.8}
			],
			"facts": []
		}`,
	}

	ext := ingest.NewExtractor(llm)
	result, err := ext.Extract(context.Background(), "Carol works at DesignCo.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(result.Relationships))
	}

	rel := result.Relationships[0]

	if rel.DateStart != nil {
		t.Errorf("expected nil date_start, got %v", rel.DateStart)
	}

	if rel.DateEnd != nil {
		t.Errorf("expected nil date_end, got %v", rel.DateEnd)
	}

	if rel.IsCurrent != nil {
		t.Errorf("expected nil is_current, got %v", rel.IsCurrent)
	}
}
