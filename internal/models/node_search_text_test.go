package models

import (
	"strings"
	"testing"
)

func TestBuildNodeSearchText(t *testing.T) {
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

	got := BuildNodeSearchText(node)
	checks := []string{
		"Persistor",
		"project",
		"Brian Colinger",
		"memory graph agents",
		"Persistent knowledge graph memory for AI agents",
		"owner: Brian Colinger",
		"Persistor is owned by Brian Colinger",
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("BuildNodeSearchText() missing %q in:\n%s", check, got)
		}
	}
}
