package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestEpisodicStore_CreateEpisodeAndEventRecord(t *testing.T) {
	base, tenantID := setupTestBase(t)
	es := store.NewEpisodicStore(base)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	projectReq := models.CreateNodeRequest{Type: "project", Label: "Persistor Phase 3"}
	_ = projectReq.Validate()
	project, err := ns.CreateNode(ctx, tenantID, projectReq)
	if err != nil {
		t.Fatalf("CreateNode(project): %v", err)
	}

	personReq := models.CreateNodeRequest{Type: "person", Label: "Brian"}
	_ = personReq.Validate()
	person, err := ns.CreateNode(ctx, tenantID, personReq)
	if err != nil {
		t.Fatalf("CreateNode(person): %v", err)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Hour).Round(time.Second)
	episodeReq := models.CreateEpisodeRequest{
		Title:                "Phase 3 kickoff",
		Summary:              "Episodic memory foundation work",
		StartedAt:            &startedAt,
		PrimaryProjectNodeID: &project.ID,
		Properties:           map[string]any{"status_reason": "roadmap"},
	}
	if err := episodeReq.Validate(); err != nil {
		t.Fatalf("CreateEpisodeRequest.Validate: %v", err)
	}

	episode, err := es.CreateEpisode(ctx, tenantID, episodeReq)
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	gotEpisode, err := es.GetEpisode(ctx, tenantID, episode.ID)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if gotEpisode.Title != episodeReq.Title {
		t.Fatalf("GetEpisode title = %q, want %q", gotEpisode.Title, episodeReq.Title)
	}
	if gotEpisode.PrimaryProjectNodeID == nil || *gotEpisode.PrimaryProjectNodeID != project.ID {
		t.Fatalf("GetEpisode primary project = %v, want %q", gotEpisode.PrimaryProjectNodeID, project.ID)
	}

	occurredAt := startedAt.Add(15 * time.Minute)
	confidence := 0.8
	eventReq := models.CreateEventRecordRequest{
		EpisodeID:  &episode.ID,
		Kind:       models.EventKindDecision,
		Title:      "Start with schema foundations",
		Summary:    "Bound Phase 3 to episode and event schema groundwork",
		OccurredAt: &occurredAt,
		Confidence: &confidence,
		Evidence: []models.EventEvidence{{
			Kind:    "note",
			Ref:     "AGENT_TASKS.md#phase-3",
			Snippet: "Introduce explicit structures for event-like memory",
		}},
		Properties: map[string]any{"status": "accepted"},
		Links: []models.CreateEventLinkRequest{
			{NodeID: person.ID, Role: "decision_maker"},
			{NodeID: project.ID, Role: "project"},
		},
	}
	if err := eventReq.Validate(); err != nil {
		t.Fatalf("CreateEventRecordRequest.Validate: %v", err)
	}

	record, err := es.CreateEventRecord(ctx, tenantID, eventReq)
	if err != nil {
		t.Fatalf("CreateEventRecord: %v", err)
	}
	if record.Kind != models.EventKindDecision {
		t.Fatalf("CreateEventRecord kind = %q, want %q", record.Kind, models.EventKindDecision)
	}

	gotRecord, err := es.GetEventRecord(ctx, tenantID, record.ID)
	if err != nil {
		t.Fatalf("GetEventRecord: %v", err)
	}
	if gotRecord.EpisodeID == nil || *gotRecord.EpisodeID != episode.ID {
		t.Fatalf("GetEventRecord episode = %v, want %q", gotRecord.EpisodeID, episode.ID)
	}
	if len(gotRecord.Evidence) != 1 || gotRecord.Evidence[0].Ref != "AGENT_TASKS.md#phase-3" {
		t.Fatalf("GetEventRecord evidence = %#v", gotRecord.Evidence)
	}

	links, err := es.ListEventLinks(ctx, tenantID, record.ID)
	if err != nil {
		t.Fatalf("ListEventLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("ListEventLinks count = %d, want 2", len(links))
	}
}
