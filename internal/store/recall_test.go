package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestRecallStore_ListEventContexts(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ctx := context.Background()
	ns := store.NewNodeStore(base)
	es := store.NewEpisodicStore(base)
	rs := store.NewRecallStore(base)

	alice, err := ns.CreateNode(ctx, tenantID, models.CreateNodeRequest{Type: "person", Label: "Alice"})
	if err != nil {
		t.Fatalf("CreateNode alice: %v", err)
	}
	project, err := ns.CreateNode(ctx, tenantID, models.CreateNodeRequest{Type: "project", Label: "Recall Pack"})
	if err != nil {
		t.Fatalf("CreateNode project: %v", err)
	}
	startedAt := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	episode, err := es.CreateEpisode(ctx, tenantID, models.CreateEpisodeRequest{Title: "Phase 3", Status: models.EpisodeStatusOpen, StartedAt: &startedAt})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	olderTime := startedAt.Add(-2 * time.Hour)
	newerTime := startedAt.Add(1 * time.Hour)
	_, err = es.CreateEventRecord(ctx, tenantID, models.CreateEventRecordRequest{
		Kind:       models.EventKindDecision,
		Title:      "Older decision",
		OccurredAt: &olderTime,
		EpisodeID:  &episode.ID,
		Links:      []models.CreateEventLinkRequest{{NodeID: alice.ID, Role: "subject"}},
	})
	if err != nil {
		t.Fatalf("CreateEventRecord older: %v", err)
	}
	newer, err := es.CreateEventRecord(ctx, tenantID, models.CreateEventRecordRequest{
		Kind:       models.EventKindTask,
		Title:      "Newer task",
		OccurredAt: &newerTime,
		EpisodeID:  &episode.ID,
		Evidence:   []models.EventEvidence{{Kind: "note", Ref: "notes#1", Snippet: "recall pack task"}},
		Links:      []models.CreateEventLinkRequest{{NodeID: alice.ID, Role: "subject"}, {NodeID: project.ID, Role: "project"}},
	})
	if err != nil {
		t.Fatalf("CreateEventRecord newer: %v", err)
	}

	contexts, err := rs.ListEventContexts(ctx, tenantID, []string{project.ID, alice.ID}, nil, 10)
	if err != nil {
		t.Fatalf("ListEventContexts: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts len = %d, want 2", len(contexts))
	}
	if contexts[0].Event.ID != newer.ID {
		t.Fatalf("first event = %q, want %q", contexts[0].Event.ID, newer.ID)
	}
	if contexts[0].Episode == nil || contexts[0].Episode.ID != episode.ID {
		t.Fatalf("episode = %#v, want %q", contexts[0].Episode, episode.ID)
	}
	if len(contexts[0].LinkedEntityIDs) != 2 {
		t.Fatalf("linked entities = %#v, want 2", contexts[0].LinkedEntityIDs)
	}

	decisionOnly, err := rs.ListEventContexts(ctx, tenantID, []string{alice.ID}, []string{models.EventKindDecision}, 10)
	if err != nil {
		t.Fatalf("ListEventContexts decision-only: %v", err)
	}
	if len(decisionOnly) != 1 || decisionOnly[0].Event.Kind != models.EventKindDecision {
		t.Fatalf("decision-only = %#v", decisionOnly)
	}
}
