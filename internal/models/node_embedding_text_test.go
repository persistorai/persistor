package models

import (
	"strings"
	"testing"
)

func TestBuildNodeEmbeddingText(t *testing.T) {
	t.Parallel()

	node := &Node{
		Type:  "project",
		Label: "Persistor",
		Properties: map[string]any{
			"summary":      "Persistent knowledge graph memory for AI agents",
			"owner":        "Brian Colinger",
			"_internal":    "ignore me",
			"search_terms": []string{"memory", "graph", "agents"},
		},
	}

	got := BuildNodeEmbeddingText(node)
	checks := []string{
		"type: project",
		"label: Persistor",
		"- owner: Brian Colinger",
		"- search_terms: memory, graph, agents",
		"- summary: Persistent knowledge graph memory for AI agents",
		"facts:",
		"Persistor is owned by Brian Colinger",
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("BuildNodeEmbeddingText() missing %q in:\n%s", check, got)
		}
	}
}

func TestNodeSummaryEmbeddingText(t *testing.T) {
	t.Parallel()

	summary := &NodeSummary{Type: "animal", Label: "Big Jerry"}
	got := summary.EmbeddingText()
	want := "type: animal\nlabel: Big Jerry"
	if got != want {
		t.Fatalf("EmbeddingText() = %q, want %q", got, want)
	}
}
