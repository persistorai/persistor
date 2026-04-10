package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

// EpisodicClient persists foundational episodic records for ingest flows.
type EpisodicClient interface {
	CreateEpisode(ctx context.Context, tenantID string, req models.CreateEpisodeRequest) (*models.Episode, error)
	CreateEventRecord(ctx context.Context, tenantID string, req models.CreateEventRecordRequest) (*models.EventRecord, error)
}

func (w *Writer) WithEpisodic(episodic EpisodicClient, tenantID string) *Writer {
	w.episodic = episodic
	w.tenantID = strings.TrimSpace(tenantID)
	return w
}

type episodicPlan struct {
	episode models.CreateEpisodeRequest
	events  []models.CreateEventRecordRequest
}

func (w *Writer) WriteEpisodic(ctx context.Context, entities []ExtractedEntity, rels []ExtractedRelationship, nodeMap map[string]string) (int, int, error) {
	if w.episodic == nil || w.tenantID == "" {
		return 0, 0, nil
	}

	plan := buildEpisodicPlan(w.source, entities, rels, nodeMap)
	if len(plan.events) == 0 {
		return 0, 0, nil
	}
	if err := plan.episode.Validate(); err != nil {
		return 0, 0, fmt.Errorf("validating episode plan: %w", err)
	}

	episode, err := w.episodic.CreateEpisode(ctx, w.tenantID, plan.episode)
	if err != nil {
		return 0, 0, fmt.Errorf("creating episode: %w", err)
	}

	createdEvents := 0
	for i := range plan.events {
		plan.events[i].EpisodeID = &episode.ID
		if err := plan.events[i].Validate(); err != nil {
			return 1, createdEvents, fmt.Errorf("validating event record %q: %w", plan.events[i].Title, err)
		}
		if _, err := w.episodic.CreateEventRecord(ctx, w.tenantID, plan.events[i]); err != nil {
			return 1, createdEvents, fmt.Errorf("creating event record %q: %w", plan.events[i].Title, err)
		}
		createdEvents++
	}

	return 1, createdEvents, nil
}

func buildEpisodicPlan(source string, entities []ExtractedEntity, rels []ExtractedRelationship, nodeMap map[string]string) episodicPlan {
	events := collectEventRecords(source, entities, rels, nodeMap)
	if len(events) == 0 {
		return episodicPlan{}
	}

	var startedAt *time.Time
	var endedAt *time.Time
	for i := range events {
		if ts := eventStartTime(events[i]); ts != nil && (startedAt == nil || ts.Before(*startedAt)) {
			copy := *ts
			startedAt = &copy
		}
		if ts := eventEndTime(events[i]); ts != nil && (endedAt == nil || ts.After(*endedAt)) {
			copy := *ts
			endedAt = &copy
		}
	}

	return episodicPlan{
		episode: models.CreateEpisodeRequest{
			Title:     episodeTitle(source),
			Summary:   fmt.Sprintf("Conservative episodic extract from %s", sourceLabel(source)),
			StartedAt: startedAt,
			EndedAt:   endedAt,
			Properties: map[string]any{
				"ingest_source": source,
				"event_count":   len(events),
			},
		},
		events: events,
	}
}

func collectEventRecords(source string, entities []ExtractedEntity, rels []ExtractedRelationship, nodeMap map[string]string) []models.CreateEventRecordRequest {
	seen := map[string]struct{}{}
	out := make([]models.CreateEventRecordRequest, 0)

	for _, ent := range entities {
		req, ok := buildEventFromEntity(source, ent, nodeMap)
		if !ok {
			continue
		}
		key := req.Kind + "|" + strings.ToLower(strings.TrimSpace(req.Title))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, req)
	}

	for _, rel := range rels {
		req, ok := buildEventFromRelationship(source, rel, nodeMap)
		if !ok {
			continue
		}
		key := req.Kind + "|" + strings.ToLower(strings.TrimSpace(req.Title))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, req)
	}

	sort.SliceStable(out, func(i, j int) bool {
		left := eventStartTime(out[i])
		right := eventStartTime(out[j])
		switch {
		case left == nil && right == nil:
			return out[i].Title < out[j].Title
		case left == nil:
			return false
		case right == nil:
			return true
		case left.Equal(*right):
			return out[i].Title < out[j].Title
		default:
			return left.Before(*right)
		}
	})

	return out
}

func buildEventFromEntity(source string, ent ExtractedEntity, nodeMap map[string]string) (models.CreateEventRecordRequest, bool) {
	kind, ok := entityEventKind(ent)
	if !ok {
		return models.CreateEventRecordRequest{}, false
	}

	links := make([]models.CreateEventLinkRequest, 0, 1)
	if nodeID, ok := nodeMap[strings.ToLower(strings.TrimSpace(ent.Name))]; ok {
		links = append(links, models.CreateEventLinkRequest{NodeID: nodeID, Role: "subject"})
	}

	occurredAt, occurredStartAt, occurredEndAt := extractTemporalBounds(ent.Properties)
	req := models.CreateEventRecordRequest{
		Kind:            kind,
		Title:           strings.TrimSpace(ent.Name),
		Summary:         strings.TrimSpace(ent.Description),
		OccurredAt:      occurredAt,
		OccurredStartAt: occurredStartAt,
		OccurredEndAt:   occurredEndAt,
		Evidence: []models.EventEvidence{{
			Kind:    "entity",
			Ref:     ent.Name,
			Snippet: strings.TrimSpace(ent.Description),
			Properties: map[string]any{
				"entity_type": ent.Type,
				"source":      source,
			},
		}},
		Properties: map[string]any{
			"ingest_source": source,
			"entity_type":   ent.Type,
		},
		Links: links,
	}
	return req, true
}

func buildEventFromRelationship(source string, rel ExtractedRelationship, nodeMap map[string]string) (models.CreateEventRecordRequest, bool) {
	if !strings.EqualFold(strings.TrimSpace(rel.Relation), "decided") {
		return models.CreateEventRecordRequest{}, false
	}

	links := make([]models.CreateEventLinkRequest, 0, 2)
	if nodeID, ok := nodeMap[strings.ToLower(strings.TrimSpace(rel.Source))]; ok {
		links = append(links, models.CreateEventLinkRequest{NodeID: nodeID, Role: "decision_maker"})
	}
	if nodeID, ok := nodeMap[strings.ToLower(strings.TrimSpace(rel.Target))]; ok {
		links = append(links, models.CreateEventLinkRequest{NodeID: nodeID, Role: "decision_topic"})
	}

	var occurredAt, occurredStartAt, occurredEndAt *time.Time
	if rel.DateStart != nil || rel.DateEnd != nil {
		occurredStartAt = parseEventTime(rel.DateStart)
		occurredEndAt = parseEventTime(rel.DateEnd)
		if occurredStartAt != nil && occurredEndAt != nil && occurredStartAt.Equal(*occurredEndAt) {
			copy := *occurredStartAt
			occurredAt = &copy
			occurredStartAt = nil
			occurredEndAt = nil
		} else if occurredStartAt != nil && rel.DateEnd == nil {
			copy := *occurredStartAt
			occurredAt = &copy
			occurredStartAt = nil
		}
	}

	confidence := rel.Confidence
	return models.CreateEventRecordRequest{
		Kind:            models.EventKindDecision,
		Title:           strings.TrimSpace(rel.Source) + " decided " + strings.TrimSpace(rel.Target),
		Summary:         strings.TrimSpace(rel.Source) + " decided " + strings.TrimSpace(rel.Target),
		OccurredAt:      occurredAt,
		OccurredStartAt: occurredStartAt,
		OccurredEndAt:   occurredEndAt,
		Confidence:      &confidence,
		Evidence: []models.EventEvidence{{
			Kind:       "relationship",
			Ref:        strings.TrimSpace(rel.Source) + "->" + strings.TrimSpace(rel.Relation) + "->" + strings.TrimSpace(rel.Target),
			Snippet:    strings.TrimSpace(rel.Source) + " decided " + strings.TrimSpace(rel.Target),
			Confidence: &confidence,
			Properties: map[string]any{"relation": rel.Relation, "source": source},
		}},
		Properties: map[string]any{"ingest_source": source, "relation": rel.Relation},
		Links:      links,
	}, true
}

func entityEventKind(ent ExtractedEntity) (string, bool) {
	typ := strings.ToLower(strings.TrimSpace(ent.Type))
	switch typ {
	case "decision":
		return models.EventKindDecision, true
	case "event":
		if kind, ok := extractExplicitEventKind(ent.Properties); ok {
			return kind, true
		}
		return models.EventKindObservation, true
	default:
		if kind, ok := extractExplicitEventKind(ent.Properties); ok {
			return kind, true
		}
		return "", false
	}
}

func extractExplicitEventKind(props map[string]any) (string, bool) {
	for _, key := range []string{"event_kind", "kind", "event_type"} {
		raw, ok := props[key]
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		switch kind {
		case models.EventKindObservation, models.EventKindConversation, models.EventKindMessage, models.EventKindDecision, models.EventKindTask, models.EventKindPromise, models.EventKindOutcome:
			return kind, true
		}
	}
	return "", false
}

func extractTemporalBounds(props map[string]any) (*time.Time, *time.Time, *time.Time) {
	for _, key := range []string{"occurred_at", "date", "timestamp"} {
		if ts := parseEventTimeValue(props[key]); ts != nil {
			copy := *ts
			return &copy, nil, nil
		}
	}
	start := firstParsedTime(props, "occurred_start_at", "started_at", "date_start")
	end := firstParsedTime(props, "occurred_end_at", "ended_at", "date_end")
	if start != nil && end != nil && start.Equal(*end) {
		copy := *start
		return &copy, nil, nil
	}
	return nil, start, end
}

func firstParsedTime(props map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		if ts := parseEventTimeValue(props[key]); ts != nil {
			return ts
		}
	}
	return nil
}

func parseEventTime(raw *string) *time.Time {
	if raw == nil {
		return nil
	}
	return parseEventTimeValue(*raw)
}

func parseEventTimeValue(raw any) *time.Time {
	s := strings.TrimSpace(fmt.Sprint(raw))
	if s == "" || s == "<nil>" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", time.RFC3339Nano}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, s); err == nil {
			utc := ts.UTC()
			return &utc
		}
	}
	return nil
}

func eventStartTime(req models.CreateEventRecordRequest) *time.Time {
	if req.OccurredAt != nil {
		return req.OccurredAt
	}
	return req.OccurredStartAt
}

func eventEndTime(req models.CreateEventRecordRequest) *time.Time {
	if req.OccurredAt != nil {
		return req.OccurredAt
	}
	if req.OccurredEndAt != nil {
		return req.OccurredEndAt
	}
	return req.OccurredStartAt
}

func episodeTitle(source string) string {
	label := sourceLabel(source)
	if label == "ingest" {
		return "Ingested episode"
	}
	return "Ingested episode: " + label
}

func sourceLabel(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "ingest"
	}
	base := filepath.Base(trimmed)
	if base == "." || base == string(filepath.Separator) {
		return trimmed
	}
	return base
}
