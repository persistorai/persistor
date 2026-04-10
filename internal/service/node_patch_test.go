package service

import (
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestNodeService_PatchNodePropertiesReembeds(t *testing.T) {
	t.Parallel()

	store := &mockNodeStore{
		patchNodeProperties: func(_ context.Context, _, _ string, _ models.PatchPropertiesRequest) (*models.Node, error) {
			return &models.Node{
				ID:    "n1",
				Type:  "project",
				Label: "Persistor",
				Properties: map[string]any{
					"summary": "Persistent knowledge graph memory",
				},
			}, nil
		},
	}

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	embedEnq := &mockEmbedEnqueuer{}
	svc := NewNodeService(store, embedEnq, nil, log)

	_, err := svc.PatchNodeProperties(context.Background(), "t1", "n1", models.PatchPropertiesRequest{
		Properties: map[string]any{"summary": "Persistent knowledge graph memory"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embedEnq.jobs) != 1 {
		t.Fatalf("expected 1 embed job, got %d", len(embedEnq.jobs))
	}
	text := embedEnq.jobs[0].Text
	checks := []string{
		"type: project",
		"label: Persistor",
		"- summary: Persistent knowledge graph memory",
		"facts:",
		"summary: Persistent knowledge graph memory",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("unexpected embedding text, missing %q in %q", check, text)
		}
	}
}
