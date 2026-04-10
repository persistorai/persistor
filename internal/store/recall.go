package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// RecallStore provides bounded event/episode lookups for recall-pack assembly.
type RecallStore struct {
	Base
}

func NewRecallStore(base Base) *RecallStore {
	return &RecallStore{Base: base}
}

func (s *RecallStore) ListEventContexts(ctx context.Context, tenantID string, nodeIDs []string, kinds []string, limit int) ([]models.RecallEventContext, error) {
	if len(nodeIDs) == 0 {
		return []models.RecallEventContext{}, nil
	}
	if limit <= 0 {
		limit = models.DefaultRecallRecentEpisodeLimit
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing recall event contexts: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	args := []any{nodeIDs}
	kindClause := ""
	limitArg := 2
	if len(kinds) > 0 {
		kindClause = " AND er.kind = ANY($2)"
		args = append(args, kinds)
		limitArg = 3
	}
	args = append(args, limit)

	query := `SELECT
		er.id, er.tenant_id, er.episode_id, er.parent_event_id, er.kind, er.title, er.summary,
		er.occurred_at, er.occurred_start_at, er.occurred_end_at, er.confidence, er.evidence,
		er.source_artifact_node_id, er.properties, er.created_at, er.updated_at,
		ep.id, ep.tenant_id, ep.title, ep.summary, ep.status, ep.started_at, ep.ended_at,
		ep.primary_project_node_id, ep.source_artifact_node_id, ep.properties, ep.created_at, ep.updated_at,
		COALESCE(array_agg(DISTINCT el.node_id) FILTER (WHERE el.node_id IS NOT NULL), '{}') AS linked_entity_ids,
		COALESCE(array_agg(DISTINCT el.role) FILTER (WHERE el.role IS NOT NULL), '{}') AS linked_roles
	FROM kg_event_records er
	LEFT JOIN kg_episodes ep
		ON ep.tenant_id = er.tenant_id AND ep.id = er.episode_id
	LEFT JOIN kg_event_links el
		ON el.tenant_id = er.tenant_id AND el.event_id = er.id AND el.node_id = ANY($1)
	WHERE er.tenant_id = current_setting('app.tenant_id')::uuid
	  AND EXISTS (
		SELECT 1 FROM kg_event_links sel
		WHERE sel.tenant_id = er.tenant_id AND sel.event_id = er.id AND sel.node_id = ANY($1)
	  )` + kindClause + `
	GROUP BY
		er.id, er.tenant_id, er.episode_id, er.parent_event_id, er.kind, er.title, er.summary,
		er.occurred_at, er.occurred_start_at, er.occurred_end_at, er.confidence, er.evidence,
		er.source_artifact_node_id, er.properties, er.created_at, er.updated_at,
		ep.id, ep.tenant_id, ep.title, ep.summary, ep.status, ep.started_at, ep.ended_at,
		ep.primary_project_node_id, ep.source_artifact_node_id, ep.properties, ep.created_at, ep.updated_at
	ORDER BY COALESCE(er.occurred_at, er.occurred_end_at, er.occurred_start_at, er.created_at) DESC,
		er.confidence DESC,
		er.id ASC
	LIMIT $` + fmt.Sprintf("%d", limitArg)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying recall event contexts: %w", err)
	}
	defer rows.Close()

	contexts := make([]models.RecallEventContext, 0, limit)
	for rows.Next() {
		ctxItem, err := scanRecallEventContextRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning recall event context: %w", err)
		}
		contexts = append(contexts, *ctxItem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recall event contexts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing recall event contexts: %w", err)
	}
	return contexts, nil
}

func scanRecallEventContextRow(scan func(dest ...any) error) (*models.RecallEventContext, error) {
	var (
		record          models.EventRecord
		recordEvidence  []byte
		recordProps     []byte
		episodeID       *string
		episode         models.Episode
		episodeProps    []byte
		linkedEntityIDs []string
		linkedRoles     []string
	)

	if err := scan(
		&record.ID,
		&record.TenantID,
		&record.EpisodeID,
		&record.ParentEventID,
		&record.Kind,
		&record.Title,
		&record.Summary,
		&record.OccurredAt,
		&record.OccurredStartAt,
		&record.OccurredEndAt,
		&record.Confidence,
		&recordEvidence,
		&record.SourceArtifactNodeID,
		&recordProps,
		&record.CreatedAt,
		&record.UpdatedAt,
		&episodeID,
		&episode.TenantID,
		&episode.Title,
		&episode.Summary,
		&episode.Status,
		&episode.StartedAt,
		&episode.EndedAt,
		&episode.PrimaryProjectNodeID,
		&episode.SourceArtifactNodeID,
		&episodeProps,
		&episode.CreatedAt,
		&episode.UpdatedAt,
		&linkedEntityIDs,
		&linkedRoles,
	); err != nil {
		return nil, err
	}
	if len(recordEvidence) > 0 {
		if err := json.Unmarshal(recordEvidence, &record.Evidence); err != nil {
			return nil, fmt.Errorf("unmarshalling recall event evidence: %w", err)
		}
	}
	if len(recordProps) > 0 {
		if err := json.Unmarshal(recordProps, &record.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling recall event properties: %w", err)
		}
	}
	if record.Evidence == nil {
		record.Evidence = []models.EventEvidence{}
	}
	if record.Properties == nil {
		record.Properties = map[string]any{}
	}

	var episodePtr *models.Episode
	if episodeID != nil {
		episode.ID = *episodeID
		if len(episodeProps) > 0 {
			if err := json.Unmarshal(episodeProps, &episode.Properties); err != nil {
				return nil, fmt.Errorf("unmarshalling recall episode properties: %w", err)
			}
		}
		if episode.Properties == nil {
			episode.Properties = map[string]any{}
		}
		episodePtr = &episode
	}

	return &models.RecallEventContext{
		Event:           record,
		Episode:         episodePtr,
		LinkedEntityIDs: linkedEntityIDs,
		LinkedRoles:     linkedRoles,
	}, nil
}
