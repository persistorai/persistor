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
      "prompt": "Who is Comet?",
      "search_mode": "text",
      "limit": 5,
      "expected_node_ids": ["comet"]
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
      "prompt": "What changed in Aurora production on Apr 1 and Apr 2?",
      "expected_labels": ["Aurora"]
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
      "prompt": "Who is Comet?"
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

func TestLoadFixtureRejectsInvalidPreferredFirstExpectation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	content := `{
  "name": "broken-fixture",
  "questions": [
    {
      "prompt": "Who owns Persistor?",
      "expected_labels": ["Persistor"],
      "preferred_first_label": "Avery Quinn"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFixture(path)
	if err == nil {
		t.Fatal("expected error for invalid preferred first expectation")
	}
}
