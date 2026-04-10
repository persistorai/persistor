package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

var recallDecisionKinds = []string{models.EventKindDecision, models.EventKindTask, models.EventKindPromise}

// RecallStore defines the narrow data access required for recall-pack assembly.
type RecallStore interface {
	GetNode(ctx context.Context, tenantID, nodeID string) (*models.Node, error)
	Neighbors(ctx context.Context, tenantID, nodeID string, limit int) (*models.NeighborResult, error)
	ListEventContexts(ctx context.Context, tenantID string, nodeIDs []string, kinds []string, limit int) ([]models.RecallEventContext, error)
}

// RecallService assembles compact deterministic recall packs for active topics.
type RecallService struct {
	store RecallStore
	log   *logrus.Logger
}

func NewRecallService(store RecallStore, log *logrus.Logger) *RecallService {
	return &RecallService{store: store, log: log}
}

func (s *RecallService) BuildRecallPack(ctx context.Context, tenantID string, req models.RecallPackRequest) (*models.RecallPack, error) {
	req = req.Normalized()
	coreNodes := make([]models.Node, 0, len(req.NodeIDs))
	seenNodeIDs := make(map[string]struct{}, len(req.NodeIDs))
	for _, nodeID := range req.NodeIDs {
		if nodeID == "" {
			continue
		}
		if _, ok := seenNodeIDs[nodeID]; ok {
			continue
		}
		seenNodeIDs[nodeID] = struct{}{}
		node, err := s.store.GetNode(ctx, tenantID, nodeID)
		if err != nil {
			return nil, err
		}
		coreNodes = append(coreNodes, *node)
	}
	sort.SliceStable(coreNodes, func(i, j int) bool {
		if coreNodes[i].Salience != coreNodes[j].Salience {
			return coreNodes[i].Salience > coreNodes[j].Salience
		}
		if coreNodes[i].Label != coreNodes[j].Label {
			return coreNodes[i].Label < coreNodes[j].Label
		}
		return coreNodes[i].ID < coreNodes[j].ID
	})

	pack := &models.RecallPack{CoreEntities: make([]models.RecallEntity, 0, len(coreNodes))}
	for _, node := range coreNodes {
		pack.CoreEntities = append(pack.CoreEntities, models.RecallEntity{ID: node.ID, Type: node.Type, Label: node.Label, Salience: node.Salience})
	}

	pack.NotableNeighbors = s.buildNeighbors(ctx, tenantID, coreNodes, req.NeighborLimit)
	pack.Contradictions = buildContradictions(coreNodes, req.ContradictionLimit)
	pack.StrongestEvidence = buildEvidence(coreNodes, req.EvidenceLimit)

	eventContexts, err := s.store.ListEventContexts(ctx, tenantID, req.NodeIDs, nil, req.RecentEpisodeLimit)
	if err != nil {
		return nil, err
	}
	pack.RecentEpisodes = buildRecentEpisodes(eventContexts, req.RecentEpisodeLimit)

	decisionContexts, err := s.store.ListEventContexts(ctx, tenantID, req.NodeIDs, recallDecisionKinds, req.OpenDecisionLimit*2)
	if err != nil {
		return nil, err
	}
	pack.OpenDecisions = buildOpenDecisions(decisionContexts, req.OpenDecisionLimit)

	s.log.WithFields(logrus.Fields{"tenant_id": tenantID, "core_entities": len(pack.CoreEntities)}).Debug("recall.build")
	return pack, nil
}

func (s *RecallService) buildNeighbors(ctx context.Context, tenantID string, coreNodes []models.Node, limit int) []models.RecallNeighbor {
	type agg struct {
		neighbor    models.Node
		relation    string
		direction   string
		weight      float64
		salience    float64
		connectedTo map[string]struct{}
		score       float64
	}
	byKey := map[string]*agg{}
	for _, node := range coreNodes {
		result, err := s.store.Neighbors(ctx, tenantID, node.ID, limit*3)
		if err != nil || result == nil {
			continue
		}
		nodeByID := map[string]models.Node{}
		for _, neighbor := range result.Nodes {
			nodeByID[neighbor.ID] = neighbor
		}
		for _, edge := range result.Edges {
			var neighborID, direction string
			switch {
			case edge.Source == node.ID:
				neighborID, direction = edge.Target, "outgoing"
			case edge.Target == node.ID:
				neighborID, direction = edge.Source, "incoming"
			default:
				continue
			}
			neighbor, ok := nodeByID[neighborID]
			if !ok {
				continue
			}
			key := neighbor.ID + "|" + edge.Relation + "|" + direction
			item, ok := byKey[key]
			if !ok {
				item = &agg{neighbor: neighbor, relation: edge.Relation, direction: direction, connectedTo: map[string]struct{}{}}
				byKey[key] = item
			}
			item.connectedTo[node.ID] = struct{}{}
			if edge.Weight > item.weight {
				item.weight = edge.Weight
			}
			if edge.Salience > item.salience {
				item.salience = edge.Salience
			}
			candidateScore := neighbor.Salience*2 + edge.Salience*3 + edge.Weight
			if candidateScore > item.score {
				item.score = candidateScore
				item.neighbor = neighbor
			}
		}
	}
	items := make([]*agg, 0, len(byKey))
	for _, item := range byKey {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].neighbor.Label != items[j].neighbor.Label {
			return items[i].neighbor.Label < items[j].neighbor.Label
		}
		if items[i].relation != items[j].relation {
			return items[i].relation < items[j].relation
		}
		return items[i].neighbor.ID < items[j].neighbor.ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]models.RecallNeighbor, 0, len(items))
	for _, item := range items {
		connectedTo := make([]string, 0, len(item.connectedTo))
		for id := range item.connectedTo {
			connectedTo = append(connectedTo, id)
		}
		sort.Strings(connectedTo)
		out = append(out, models.RecallNeighbor{
			Node:        models.RecallEntity{ID: item.neighbor.ID, Type: item.neighbor.Type, Label: item.neighbor.Label, Salience: item.neighbor.Salience},
			Relation:    item.relation,
			Direction:   item.direction,
			Weight:      item.weight,
			Salience:    item.salience,
			ConnectedTo: connectedTo,
		})
	}
	return out
}

func buildRecentEpisodes(contexts []models.RecallEventContext, limit int) []models.RecallEpisode {
	out := make([]models.RecallEpisode, 0, minInt(limit, len(contexts)))
	for _, item := range contexts {
		recall := models.RecallEpisode{
			EventID:         item.Event.ID,
			Kind:            item.Event.Kind,
			Title:           item.Event.Title,
			Summary:         item.Event.Summary,
			OccurredAt:      recallOccurredAt(item.Event),
			Confidence:      item.Event.Confidence,
			LinkedEntityIDs: sortedCopy(item.LinkedEntityIDs),
		}
		if item.Episode != nil {
			recall.EpisodeID = &item.Episode.ID
			recall.EpisodeTitle = item.Episode.Title
			recall.EpisodeStatus = item.Episode.Status
		}
		out = append(out, recall)
		if len(out) == limit {
			break
		}
	}
	return out
}

func buildOpenDecisions(contexts []models.RecallEventContext, limit int) []models.RecallDecision {
	out := make([]models.RecallDecision, 0, minInt(limit, len(contexts)))
	for _, item := range contexts {
		status := openDecisionStatus(item)
		if status == "" {
			continue
		}
		recall := models.RecallDecision{
			EventID:         item.Event.ID,
			Kind:            item.Event.Kind,
			Title:           item.Event.Title,
			Summary:         item.Event.Summary,
			OccurredAt:      recallOccurredAt(item.Event),
			Status:          status,
			LinkedEntityIDs: sortedCopy(item.LinkedEntityIDs),
		}
		if item.Episode != nil {
			recall.EpisodeID = &item.Episode.ID
			recall.EpisodeTitle = item.Episode.Title
		}
		out = append(out, recall)
		if len(out) == limit {
			break
		}
	}
	return out
}

func openDecisionStatus(item models.RecallEventContext) string {
	for _, raw := range []any{item.Event.Properties["status"], item.Event.Properties["state"], item.Event.Properties["decision_status"]} {
		if status := normalizeOpenStatus(raw); status != "" {
			return status
		}
	}
	if item.Episode != nil && item.Episode.Status == models.EpisodeStatusOpen {
		return models.EpisodeStatusOpen
	}
	return ""
}

func normalizeOpenStatus(raw any) string {
	status, ok := raw.(string)
	if !ok {
		return ""
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "open", "pending", "active", "unresolved", "todo", "in_progress":
		return status
	case "closed", "resolved", "done", "cancelled", "canceled":
		return ""
	default:
		return ""
	}
}

func buildContradictions(nodes []models.Node, limit int) []models.RecallContradiction {
	items := make([]models.RecallContradiction, 0, limit)
	for _, node := range nodes {
		beliefs := decodeFactBeliefs(node.Properties)
		for property, belief := range beliefs {
			if belief.Status != models.FactBeliefStatusContested && belief.Status != models.FactBeliefStatusSuperseded {
				continue
			}
			item := models.RecallContradiction{
				EntityID:            node.ID,
				EntityLabel:         node.Label,
				Property:            property,
				Status:              belief.Status,
				PreferredValue:      belief.PreferredValue,
				PreferredConfidence: belief.PreferredConfidence,
				EvidenceCount:       belief.EvidenceCount,
			}
			for _, claim := range belief.Claims {
				if !claim.Preferred {
					item.AlternateValue = claim.Value
					break
				}
			}
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if contradictionRank(items[i].Status) != contradictionRank(items[j].Status) {
			return contradictionRank(items[i].Status) < contradictionRank(items[j].Status)
		}
		if items[i].EvidenceCount != items[j].EvidenceCount {
			return items[i].EvidenceCount > items[j].EvidenceCount
		}
		if items[i].EntityLabel != items[j].EntityLabel {
			return items[i].EntityLabel < items[j].EntityLabel
		}
		return items[i].Property < items[j].Property
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func contradictionRank(status string) int {
	if status == models.FactBeliefStatusContested {
		return 0
	}
	return 1
}

func buildEvidence(nodes []models.Node, limit int) []models.RecallEvidence {
	type scoredEvidence struct {
		models.RecallEvidence
		score float64
	}
	items := make([]scoredEvidence, 0, limit*2)
	for _, node := range nodes {
		evidenceByProperty := decodeFactEvidence(node.Properties)
		for property, entries := range evidenceByProperty {
			for _, entry := range entries {
				confidence := 0.0
				if entry.Confidence != nil {
					confidence = *entry.Confidence
				}
				observedAt := parseEvidenceTime(entry.Timestamp)
				snippet := ""
				if entry.PreviousValue != nil && entry.ConflictsWithPrior {
					snippet = "conflicts with prior value"
				}
				items = append(items, scoredEvidence{
					RecallEvidence: models.RecallEvidence{
						EntityID:    node.ID,
						EntityLabel: node.Label,
						Property:    property,
						Value:       entry.Value,
						Source:      entry.Source,
						Snippet:     snippet,
						Confidence:  confidence,
						ObservedAt:  observedAt,
					},
					score: confidence + node.Salience/100,
				})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if compareTimePtr(items[i].ObservedAt, items[j].ObservedAt) != 0 {
			return compareTimePtr(items[i].ObservedAt, items[j].ObservedAt) > 0
		}
		if items[i].EntityLabel != items[j].EntityLabel {
			return items[i].EntityLabel < items[j].EntityLabel
		}
		return items[i].Property < items[j].Property
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]models.RecallEvidence, 0, len(items))
	for _, item := range items {
		out = append(out, item.RecallEvidence)
	}
	return out
}

func decodeFactBeliefs(props map[string]any) map[string]models.FactBeliefState {
	raw, ok := props[models.FactBeliefsProperty]
	if !ok {
		return map[string]models.FactBeliefState{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return map[string]models.FactBeliefState{}
	}
	var out map[string]models.FactBeliefState
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return map[string]models.FactBeliefState{}
	}
	return out
}

func decodeFactEvidence(props map[string]any) map[string][]models.FactEvidence {
	raw, ok := props[models.FactEvidenceProperty]
	if !ok {
		return map[string][]models.FactEvidence{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return map[string][]models.FactEvidence{}
	}
	var out map[string][]models.FactEvidence
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return map[string][]models.FactEvidence{}
	}
	return out
}

func recallOccurredAt(event models.EventRecord) *time.Time {
	for _, ts := range []*time.Time{event.OccurredAt, event.OccurredEndAt, event.OccurredStartAt} {
		if ts != nil {
			return ts
		}
	}
	return &event.CreatedAt
}

func parseEvidenceTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed
		}
	}
	return nil
}

func compareTimePtr(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	case a.Equal(*b):
		return 0
	case a.After(*b):
		return 1
	default:
		return -1
	}
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
