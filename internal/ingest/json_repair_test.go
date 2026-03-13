package ingest_test

import (
	"encoding/json"
	"testing"

	"github.com/persistorai/persistor/internal/ingest"
)

func TestRepairJSON_CleanPassthrough(t *testing.T) {
	clean := `{"entities": [], "relationships": [], "facts": []}`

	result := ingest.RepairJSON(clean)

	if result != clean {
		t.Errorf("expected clean JSON to pass through unchanged, got %q", result)
	}
}

func TestRepairJSON_StripMarkdownFences(t *testing.T) {
	fenced := "```json\n{\"entities\": [], \"relationships\": [], \"facts\": []}\n```"

	result := ingest.RepairJSON(fenced)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON after stripping fences, got %q", result)
	}
}

func TestRepairJSON_StripPlainFences(t *testing.T) {
	fenced := "```\n{\"entities\": []}\n```"

	result := ingest.RepairJSON(fenced)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON, got %q", result)
	}
}

func TestRepairJSON_TruncatedJSON(t *testing.T) {
	truncated := `{"entities": [{"name": "Alice", "type": "person"}], "relationships": [`

	result := ingest.RepairJSON(truncated)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON after repair, got %q", result)
	}
}

func TestRepairJSON_TrailingCommas(t *testing.T) {
	withCommas := `{"entities": [{"name": "Alice",}], "relationships": [],}`

	result := ingest.RepairJSON(withCommas)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON after removing trailing commas, got %q", result)
	}
}

func TestRepairJSON_ExtractFromSurroundingText(t *testing.T) {
	wrapped := `Here is the JSON output:
{"entities": [], "relationships": [], "facts": []}
I hope this helps!`

	result := ingest.RepairJSON(wrapped)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON extracted from text, got %q", result)
	}
}

func TestRepairJSON_EmptyInput(t *testing.T) {
	result := ingest.RepairJSON("")

	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestRepairJSON_GarbageInput(t *testing.T) {
	garbage := "this is not json at all"

	result := ingest.RepairJSON(garbage)

	if result != garbage {
		t.Errorf("expected garbage to return as-is, got %q", result)
	}
}

func TestRepairJSON_NestedTruncation(t *testing.T) {
	truncated := `{"entities": [{"name": "Bob", "type": "person", "properties": {"role": "eng"`

	result := ingest.RepairJSON(truncated)

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON after nested repair, got %q", result)
	}
}
