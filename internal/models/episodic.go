package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	EpisodeStatusOpen   = "open"
	EpisodeStatusClosed = "closed"

	EventKindObservation  = "observation"
	EventKindConversation = "conversation"
	EventKindMessage      = "message"
	EventKindDecision     = "decision"
	EventKindTask         = "task"
	EventKindPromise      = "promise"
	EventKindOutcome      = "outcome"
)

var allowedEpisodeStatuses = map[string]struct{}{
	EpisodeStatusOpen:   {},
	EpisodeStatusClosed: {},
}

var allowedEventKinds = map[string]struct{}{
	EventKindObservation:  {},
	EventKindConversation: {},
	EventKindMessage:      {},
	EventKindDecision:     {},
	EventKindTask:         {},
	EventKindPromise:      {},
	EventKindOutcome:      {},
}

// Episode groups related event-like memory into a durable timeline container.
type Episode struct {
	ID                   string         `json:"id"`
	TenantID             uuid.UUID      `json:"-"`
	Title                string         `json:"title"`
	Summary              string         `json:"summary,omitempty"`
	Status               string         `json:"status"`
	StartedAt            *time.Time     `json:"started_at,omitempty"`
	EndedAt              *time.Time     `json:"ended_at,omitempty"`
	PrimaryProjectNodeID *string        `json:"primary_project_node_id,omitempty"`
	SourceArtifactNodeID *string        `json:"source_artifact_node_id,omitempty"`
	Properties           map[string]any `json:"properties,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// EventEvidence captures a bounded evidence pointer for an episodic record.
type EventEvidence struct {
	Kind       string         `json:"kind,omitempty"`
	Ref        string         `json:"ref,omitempty"`
	Snippet    string         `json:"snippet,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	CapturedAt *time.Time     `json:"captured_at,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// EventRecord stores a first-class event-like memory item.
type EventRecord struct {
	ID                   string          `json:"id"`
	TenantID             uuid.UUID       `json:"-"`
	EpisodeID            *string         `json:"episode_id,omitempty"`
	ParentEventID        *string         `json:"parent_event_id,omitempty"`
	Kind                 string          `json:"kind"`
	Title                string          `json:"title"`
	Summary              string          `json:"summary,omitempty"`
	OccurredAt           *time.Time      `json:"occurred_at,omitempty"`
	OccurredStartAt      *time.Time      `json:"occurred_start_at,omitempty"`
	OccurredEndAt        *time.Time      `json:"occurred_end_at,omitempty"`
	Confidence           float64         `json:"confidence"`
	Evidence             []EventEvidence `json:"evidence,omitempty"`
	SourceArtifactNodeID *string         `json:"source_artifact_node_id,omitempty"`
	Properties           map[string]any  `json:"properties,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// EventLink connects an episodic record to an existing graph node.
type EventLink struct {
	EventID   string    `json:"event_id"`
	NodeID    string    `json:"node_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateEpisodeRequest creates a new episode.
type CreateEpisodeRequest struct {
	ID                   string         `json:"id"`
	Title                string         `json:"title"`
	Summary              string         `json:"summary,omitempty"`
	Status               string         `json:"status,omitempty"`
	StartedAt            *time.Time     `json:"started_at,omitempty"`
	EndedAt              *time.Time     `json:"ended_at,omitempty"`
	PrimaryProjectNodeID *string        `json:"primary_project_node_id,omitempty"`
	SourceArtifactNodeID *string        `json:"source_artifact_node_id,omitempty"`
	Properties           map[string]any `json:"properties,omitempty"`
}

func (r *CreateEpisodeRequest) Validate() error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if len(r.ID) > 255 {
		return ErrFieldTooLong("id", 255)
	}
	if r.Title == "" {
		return fmt.Errorf("title: %w", ErrMissingLabel)
	}
	if len(r.Title) > 10000 {
		return ErrFieldTooLong("title", 10000)
	}
	if r.Status == "" {
		r.Status = EpisodeStatusOpen
	}
	if _, ok := allowedEpisodeStatuses[r.Status]; !ok {
		return fmt.Errorf("status must be one of: open, closed")
	}
	if r.StartedAt != nil && r.EndedAt != nil && r.EndedAt.Before(*r.StartedAt) {
		return fmt.Errorf("ended_at must be on or after started_at")
	}
	if r.Properties != nil {
		data, err := json.Marshal(r.Properties)
		if err != nil {
			return fmt.Errorf("invalid properties: %w", err)
		}
		if len(data) > 65536 {
			return ErrFieldTooLong("properties", 65536)
		}
	}
	return nil
}

// CreateEventRecordRequest creates a first-class episodic record.
type CreateEventRecordRequest struct {
	ID                   string                   `json:"id"`
	EpisodeID            *string                  `json:"episode_id,omitempty"`
	ParentEventID        *string                  `json:"parent_event_id,omitempty"`
	Kind                 string                   `json:"kind"`
	Title                string                   `json:"title"`
	Summary              string                   `json:"summary,omitempty"`
	OccurredAt           *time.Time               `json:"occurred_at,omitempty"`
	OccurredStartAt      *time.Time               `json:"occurred_start_at,omitempty"`
	OccurredEndAt        *time.Time               `json:"occurred_end_at,omitempty"`
	Confidence           *float64                 `json:"confidence,omitempty"`
	Evidence             []EventEvidence          `json:"evidence,omitempty"`
	SourceArtifactNodeID *string                  `json:"source_artifact_node_id,omitempty"`
	Properties           map[string]any           `json:"properties,omitempty"`
	Links                []CreateEventLinkRequest `json:"links,omitempty"`
}

// CreateEventLinkRequest links an event to an existing graph node.
type CreateEventLinkRequest struct {
	NodeID string `json:"node_id"`
	Role   string `json:"role"`
}

func (r *CreateEventRecordRequest) Validate() error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if len(r.ID) > 255 {
		return ErrFieldTooLong("id", 255)
	}
	if _, ok := allowedEventKinds[r.Kind]; !ok {
		return fmt.Errorf("kind must be one of: observation, conversation, message, decision, task, promise, outcome")
	}
	if r.Title == "" {
		return fmt.Errorf("title: %w", ErrMissingLabel)
	}
	if len(r.Title) > 10000 {
		return ErrFieldTooLong("title", 10000)
	}
	confidence := 1.0
	if r.Confidence != nil {
		confidence = *r.Confidence
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	if r.OccurredStartAt != nil && r.OccurredEndAt != nil && r.OccurredEndAt.Before(*r.OccurredStartAt) {
		return fmt.Errorf("occurred_end_at must be on or after occurred_start_at")
	}
	if r.Properties != nil {
		data, err := json.Marshal(r.Properties)
		if err != nil {
			return fmt.Errorf("invalid properties: %w", err)
		}
		if len(data) > 65536 {
			return ErrFieldTooLong("properties", 65536)
		}
	}
	if len(r.Evidence) > 0 {
		data, err := json.Marshal(r.Evidence)
		if err != nil {
			return fmt.Errorf("invalid evidence: %w", err)
		}
		if len(data) > 65536 {
			return ErrFieldTooLong("evidence", 65536)
		}
	}
	for _, link := range r.Links {
		if link.NodeID == "" {
			return ErrMissingTarget
		}
		if link.Role == "" {
			return ErrMissingRelation
		}
	}
	return nil
}
