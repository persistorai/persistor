package models

import (
	"strings"
	"testing"
)

func TestBuildNodeFactText(t *testing.T) {
	t.Parallel()

	node := &Node{
		Label: "Persistor",
		Properties: map[string]any{
			"owner":        "Brian personally",
			"not_owned_by": "Dirt Road Systems",
			"policy":       "no soft deletes",
		},
	}

	text := BuildNodeFactText(node)
	checks := []string{
		"owner: Brian personally",
		"Persistor is owned by Brian personally",
		"not_owned_by: Dirt Road Systems",
		"Persistor is not owned by Dirt Road Systems",
		"policy: no soft deletes",
		"Persistor policy: no soft deletes",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("expected fact text to contain %q, got %q", check, text)
		}
	}
}
