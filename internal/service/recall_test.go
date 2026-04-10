package service

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func TestRecallService_BuildRecallPack(t *testing.T) {
	now := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)
	confidenceHigh := 0.95
	confidenceMid := 0.72
	store := &mockRecallStore{
		getNode: func(_ context.Context, _, nodeID string) (*models.Node, error) {
			switch nodeID {
			case "topic-a":
				return &models.Node{ID: "topic-a", Type: "person", Label: "Alice", Salience: 80, Properties: map[string]any{
					models.FactBeliefsProperty:  map[string]any{"city": map[string]any{"preferred_value": "Chicago", "preferred_confidence": 0.91, "evidence_count": 2, "status": models.FactBeliefStatusContested, "claims": []map[string]any{{"value": "Chicago", "preferred": true}, {"value": "Austin"}}}},
					models.FactEvidenceProperty: map[string]any{"city": []map[string]any{{"value": "Chicago", "source": "crm", "timestamp": "2026-04-09T10:00:00Z", "confidence": confidenceHigh}, {"value": "Austin", "source": "chat", "timestamp": "2026-04-08T09:00:00Z", "confidence": confidenceMid, "conflicts_with_prior": true}}},
				}}, nil
			case "topic-b":
				return &models.Node{ID: "topic-b", Type: "project", Label: "Phase 3", Salience: 60, Properties: map[string]any{}}, nil
			default:
				return nil, models.ErrNodeNotFound
			}
		},
		neighbors: func(_ context.Context, _, nodeID string, _ int) (*models.NeighborResult, error) {
			switch nodeID {
			case "topic-a":
				return &models.NeighborResult{
					Nodes: []models.Node{{ID: "node-1", Type: "city", Label: "Chicago", Salience: 70}, {ID: "node-2", Type: "doc", Label: "Decision Log", Salience: 50}},
					Edges: []models.Edge{{Source: "topic-a", Target: "node-1", Relation: "lives_in", Weight: 1.0, Salience: 12}, {Source: "node-2", Target: "topic-a", Relation: "documents", Weight: 0.8, Salience: 6}},
				}, nil
			case "topic-b":
				return &models.NeighborResult{
					Nodes: []models.Node{{ID: "node-2", Type: "doc", Label: "Decision Log", Salience: 50}, {ID: "node-3", Type: "project", Label: "Roadmap", Salience: 40}},
					Edges: []models.Edge{{Source: "topic-b", Target: "node-2", Relation: "tracked_in", Weight: 1.2, Salience: 9}, {Source: "topic-b", Target: "node-3", Relation: "depends_on", Weight: 0.5, Salience: 4}},
				}, nil
			default:
				return &models.NeighborResult{}, nil
			}
		},
		listEventContexts: func(_ context.Context, _ string, _ []string, kinds []string, _ int) ([]models.RecallEventContext, error) {
			if len(kinds) == 0 {
				episodeID := "ep-1"
				return []models.RecallEventContext{{
					Event:           models.EventRecord{ID: "ev-2", Kind: models.EventKindMessage, Title: "Status update", Summary: "Shared progress update", OccurredAt: timePtr(now.Add(-1 * time.Hour)), Confidence: 0.8, CreatedAt: now.Add(-1 * time.Hour)},
					Episode:         &models.Episode{ID: episodeID, Title: "Sprint sync", Status: models.EpisodeStatusOpen},
					LinkedEntityIDs: []string{"topic-a", "topic-b"},
				}, {
					Event:           models.EventRecord{ID: "ev-1", Kind: models.EventKindDecision, Title: "Choose recall slice", Summary: "Keep scope tight", OccurredAt: timePtr(now.Add(-2 * time.Hour)), Confidence: 0.9, CreatedAt: now.Add(-2 * time.Hour)},
					Episode:         &models.Episode{ID: "ep-2", Title: "Phase 3 planning", Status: models.EpisodeStatusOpen},
					LinkedEntityIDs: []string{"topic-b"},
				}}, nil
			}
			return []models.RecallEventContext{{
				Event:           models.EventRecord{ID: "ev-3", Kind: models.EventKindDecision, Title: "Pending packaging decision", Summary: "Need final wording", OccurredAt: timePtr(now.Add(-30 * time.Minute)), CreatedAt: now.Add(-30 * time.Minute), Properties: map[string]any{"status": "pending"}},
				Episode:         &models.Episode{ID: "ep-3", Title: "Open decisions", Status: models.EpisodeStatusOpen},
				LinkedEntityIDs: []string{"topic-a"},
			}, {
				Event:           models.EventRecord{ID: "ev-closed", Kind: models.EventKindDecision, Title: "Closed item", CreatedAt: now.Add(-4 * time.Hour), Properties: map[string]any{"status": "closed"}},
				Episode:         &models.Episode{ID: "ep-4", Title: "Closed decisions", Status: models.EpisodeStatusClosed},
				LinkedEntityIDs: []string{"topic-b"},
			}}, nil
		},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	svc := NewRecallService(store, log)

	pack, err := svc.BuildRecallPack(context.Background(), "tenant-1", models.RecallPackRequest{NodeIDs: []string{"topic-b", "topic-a", "topic-a"}})
	if err != nil {
		t.Fatalf("BuildRecallPack: %v", err)
	}
	if len(pack.CoreEntities) != 2 || pack.CoreEntities[0].ID != "topic-a" {
		t.Fatalf("core entities = %#v", pack.CoreEntities)
	}
	if len(pack.NotableNeighbors) == 0 || pack.NotableNeighbors[0].Node.ID != "node-1" {
		t.Fatalf("notable neighbors = %#v", pack.NotableNeighbors)
	}
	if len(pack.NotableNeighbors) < 2 || pack.NotableNeighbors[1].Node.ID != "node-2" {
		t.Fatalf("second notable neighbor = %#v", pack.NotableNeighbors)
	}
	if len(pack.RecentEpisodes) != 2 || pack.RecentEpisodes[0].EventID != "ev-2" {
		t.Fatalf("recent episodes = %#v", pack.RecentEpisodes)
	}
	if len(pack.OpenDecisions) != 1 || pack.OpenDecisions[0].EventID != "ev-3" {
		t.Fatalf("open decisions = %#v", pack.OpenDecisions)
	}
	if len(pack.Contradictions) != 1 || pack.Contradictions[0].Property != "city" {
		t.Fatalf("contradictions = %#v", pack.Contradictions)
	}
	if len(pack.StrongestEvidence) != 2 || pack.StrongestEvidence[0].Value != "Chicago" {
		t.Fatalf("strongest evidence = %#v", pack.StrongestEvidence)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

type mockRecallStore struct {
	getNode           func(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	neighbors         func(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	listEventContexts func(ctx context.Context, tenantID string, nodeIDs []string, kinds []string, limit int) ([]models.RecallEventContext, error)
}

func (m *mockRecallStore) GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error) {
	return m.getNode(ctx, tenantID, nodeID)
}

func (m *mockRecallStore) Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error) {
	return m.neighbors(ctx, tenantID, nodeID, limit)
}

func (m *mockRecallStore) ListEventContexts(ctx context.Context, tenantID string, nodeIDs []string, kinds []string, limit int) ([]models.RecallEventContext, error) {
	return m.listEventContexts(ctx, tenantID, nodeIDs, kinds, limit)
}
