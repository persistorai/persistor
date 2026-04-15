package models

import "time"

const (
	DefaultRecallNeighborLimit      = 4
	DefaultRecallRecentEpisodeLimit = 4
	DefaultRecallOpenDecisionLimit  = 3
	DefaultRecallContradictionLimit = 3
	DefaultRecallEvidenceLimit      = 4
	maxRecallSectionLimit           = 10
)

// RecallPackRequest configures bounded recall-pack assembly for one or more active topics.
type RecallPackRequest struct {
	NodeIDs            []string `json:"node_ids"`
	NeighborLimit      int      `json:"neighbor_limit,omitempty"`
	RecentEpisodeLimit int      `json:"recent_episode_limit,omitempty"`
	OpenDecisionLimit  int      `json:"open_decision_limit,omitempty"`
	ContradictionLimit int      `json:"contradiction_limit,omitempty"`
	EvidenceLimit      int      `json:"evidence_limit,omitempty"`
}

func (r RecallPackRequest) Normalized() RecallPackRequest {
	r.NeighborLimit = normalizeRecallLimit(r.NeighborLimit, DefaultRecallNeighborLimit)
	r.RecentEpisodeLimit = normalizeRecallLimit(r.RecentEpisodeLimit, DefaultRecallRecentEpisodeLimit)
	r.OpenDecisionLimit = normalizeRecallLimit(r.OpenDecisionLimit, DefaultRecallOpenDecisionLimit)
	r.ContradictionLimit = normalizeRecallLimit(r.ContradictionLimit, DefaultRecallContradictionLimit)
	r.EvidenceLimit = normalizeRecallLimit(r.EvidenceLimit, DefaultRecallEvidenceLimit)
	return r
}

func normalizeRecallLimit(v, def int) int {
	if v <= 0 {
		return def
	}
	if v > maxRecallSectionLimit {
		return maxRecallSectionLimit
	}
	return v
}

// RecallPack is a compact deterministic summary for one or more active topics.
type RecallPack struct {
	CoreEntities      []RecallEntity        `json:"core_entities,omitempty"`
	NotableNeighbors  []RecallNeighbor      `json:"notable_neighbors,omitempty"`
	RecentEpisodes    []RecallEpisode       `json:"recent_episodes,omitempty"`
	OpenDecisions     []RecallDecision      `json:"open_decisions,omitempty"`
	Contradictions    []RecallContradiction `json:"contradictions,omitempty"`
	StrongestEvidence []RecallEvidence      `json:"strongest_evidence,omitempty"`
}

// RecallEntity is a compact entity summary for a pack root.
type RecallEntity struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Label    string  `json:"label"`
	Salience float64 `json:"salience_score,omitempty"`
}

// RecallNeighbor captures a notable adjacent entity with the connecting relation.
type RecallNeighbor struct {
	Node        RecallEntity `json:"node"`
	Relation    string       `json:"relation"`
	Direction   string       `json:"direction"`
	Weight      float64      `json:"weight,omitempty"`
	Salience    float64      `json:"salience_score,omitempty"`
	ConnectedTo []string     `json:"connected_to,omitempty"`
}

// RecallEpisode captures a recent linked event and optional episode context.
type RecallEpisode struct {
	EventID             string     `json:"event_id"`
	Kind                string     `json:"kind"`
	Title               string     `json:"title"`
	Summary             string     `json:"summary,omitempty"`
	OccurredAt          *time.Time `json:"occurred_at,omitempty"`
	Confidence          float64    `json:"confidence,omitempty"`
	EpisodeID           *string    `json:"episode_id,omitempty"`
	EpisodeTitle        string     `json:"episode_title,omitempty"`
	EpisodeStatus       string     `json:"episode_status,omitempty"`
	LinkedEntityIDs     []string   `json:"linked_entity_ids,omitempty"`
	EventCount          int        `json:"event_count,omitempty"`
	EventKinds          []string   `json:"event_kinds,omitempty"`
	OpenItemCount       int        `json:"open_item_count,omitempty"`
	LatestOutcomeTitle  string     `json:"latest_outcome_title,omitempty"`
	LatestOutcomeStatus string     `json:"latest_outcome_status,omitempty"`
}

// RecallDecision captures a still-open decision-like event.
type RecallDecision struct {
	EventID         string     `json:"event_id"`
	Kind            string     `json:"kind"`
	Title           string     `json:"title"`
	Summary         string     `json:"summary,omitempty"`
	OccurredAt      *time.Time `json:"occurred_at,omitempty"`
	EpisodeID       *string    `json:"episode_id,omitempty"`
	EpisodeTitle    string     `json:"episode_title,omitempty"`
	Status          string     `json:"status,omitempty"`
	LinkedEntityIDs []string   `json:"linked_entity_ids,omitempty"`
}

// RecallContradiction captures contested or superseded claims.
type RecallContradiction struct {
	EntityID            string  `json:"entity_id"`
	EntityLabel         string  `json:"entity_label"`
	Property            string  `json:"property"`
	Status              string  `json:"status"`
	PreferredValue      any     `json:"preferred_value,omitempty"`
	AlternateValue      any     `json:"alternate_value,omitempty"`
	PreferredConfidence float64 `json:"preferred_confidence,omitempty"`
	EvidenceCount       int     `json:"evidence_count,omitempty"`
}

// RecallEvidence captures a compact evidence pointer.
type RecallEvidence struct {
	EntityID    string     `json:"entity_id,omitempty"`
	EntityLabel string     `json:"entity_label,omitempty"`
	Property    string     `json:"property,omitempty"`
	Value       any        `json:"value,omitempty"`
	Source      string     `json:"source,omitempty"`
	Snippet     string     `json:"snippet,omitempty"`
	Confidence  float64    `json:"confidence,omitempty"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	EventID     string     `json:"event_id,omitempty"`
	EventTitle  string     `json:"event_title,omitempty"`
}

// RecallEventContext is a compact event+episode join used by store-backed recall assembly.
type RecallEventContext struct {
	Event           EventRecord `json:"event"`
	Episode         *Episode    `json:"episode,omitempty"`
	LinkedEntityIDs []string    `json:"linked_entity_ids,omitempty"`
	LinkedRoles     []string    `json:"linked_roles,omitempty"`
}
