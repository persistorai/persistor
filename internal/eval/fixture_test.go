package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	content := `{
  "name": "test-fixture",
  "questions": [
    {
      "prompt": "Who is Big Jerry?",
      "search_mode": "hybrid",
      "limit": 5,
      "expected_node_ids": ["big-jerry"]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	fixture, err := LoadFixture(path)
	if err != nil {
		t.Fatalf("LoadFixture returned error: %v", err)
	}

	if fixture.Name != "test-fixture" {
		t.Fatalf("expected fixture name test-fixture, got %q", fixture.Name)
	}
	if len(fixture.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(fixture.Questions))
	}
}

func TestLoadFixtureAllowsLabelOnlyExpectations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	content := `{
  "name": "label-only-fixture",
  "questions": [
    {
      "prompt": "What changed in DeerPrint production on Apr 1 and Apr 2?",
      "expected_labels": ["DeerPrint"]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	fixture, err := LoadFixture(path)
	if err != nil {
		t.Fatalf("LoadFixture returned error: %v", err)
	}
	if len(fixture.Questions) != 1 || len(fixture.Questions[0].ExpectedLabels) != 1 {
		t.Fatalf("expected label-only fixture to load, got %#v", fixture)
	}
}

func TestLoadFixtureRejectsMissingExpectations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	content := `{
  "name": "broken-fixture",
  "questions": [
    {
      "prompt": "Who is Big Jerry?"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFixture(path)
	if err == nil {
		t.Fatal("expected error for missing expectations")
	}
}
