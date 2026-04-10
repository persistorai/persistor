package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/persistorai/persistor/internal/models"
)

const episodeColumns = `id, tenant_id, title, summary, status, started_at, ended_at,
	primary_project_node_id, source_artifact_node_id, properties, created_at, updated_at`

const eventRecordColumns = `id, tenant_id, episode_id, parent_event_id, kind, title, summary,
	occurred_at, occurred_start_at, occurred_end_at, confidence, evidence,
	source_artifact_node_id, properties, created_at, updated_at`

// EpisodicStore persists episode and event-record foundations.
type EpisodicStore struct {
	Base
}

func NewEpisodicStore(base Base) *EpisodicStore {
	return &EpisodicStore{Base: base}
}

func (s *EpisodicStore) CreateEpisode(ctx context.Context, tenantID string, req models.CreateEpisodeRequest) (*models.Episode, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating episode: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}

	row := tx.QueryRow(ctx, `INSERT INTO kg_episodes (
		id, tenant_id, title, summary, status, started_at, ended_at,
		primary_project_node_id, source_artifact_node_id, properties
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	RETURNING `+episodeColumns,
		req.ID, tenantID, req.Title, req.Summary, req.Status, req.StartedAt, req.EndedAt,
		req.PrimaryProjectNodeID, req.SourceArtifactNodeID, props)

	episode, err := scanEpisode(row.Scan)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}
		return nil, fmt.Errorf("scanning created episode: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing create episode: %w", err)
	}

	s.notify("kg_episodes", "insert", tenantID)
	return episode, nil
}

func (s *EpisodicStore) GetEpisode(ctx context.Context, tenantID, episodeID string) (*models.Episode, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting episode: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `SELECT `+episodeColumns+` FROM kg_episodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`, episodeID)
	episode, err := scanEpisode(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEpisodeNotFound
		}
		return nil, fmt.Errorf("scanning episode: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing get episode: %w", err)
	}
	return episode, nil
}

func (s *EpisodicStore) CreateEventRecord(ctx context.Context, tenantID string, req models.CreateEventRecordRequest) (*models.EventRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating event record: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	evidence := req.Evidence
	if evidence == nil {
		evidence = []models.EventEvidence{}
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("marshalling evidence: %w", err)
	}

	row := tx.QueryRow(ctx, `INSERT INTO kg_event_records (
		id, tenant_id, episode_id, parent_event_id, kind, title, summary,
		occurred_at, occurred_start_at, occurred_end_at, confidence, evidence,
		source_artifact_node_id, properties
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	RETURNING `+eventRecordColumns,
		req.ID, tenantID, req.EpisodeID, req.ParentEventID, req.Kind, req.Title, req.Summary,
		req.OccurredAt, req.OccurredStartAt, req.OccurredEndAt, confidence, evidenceJSON,
		req.SourceArtifactNodeID, props)

	record, err := scanEventRecord(row.Scan)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}
		return nil, fmt.Errorf("scanning created event record: %w", err)
	}

	if len(req.Links) > 0 {
		batch := &pgx.Batch{}
		for _, link := range req.Links {
			batch.Queue(`INSERT INTO kg_event_links (tenant_id, event_id, node_id, role) VALUES ($1, $2, $3, $4)`, tenantID, req.ID, link.NodeID, link.Role)
		}
		results := tx.SendBatch(ctx, batch)
		if err := results.Close(); err != nil {
			return nil, fmt.Errorf("creating event links: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing create event record: %w", err)
	}

	s.notify("kg_event_records", "insert", tenantID)
	return record, nil
}

func (s *EpisodicStore) GetEventRecord(ctx context.Context, tenantID, eventID string) (*models.EventRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting event record: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `SELECT `+eventRecordColumns+` FROM kg_event_records WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`, eventID)
	record, err := scanEventRecord(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEventRecordNotFound
		}
		return nil, fmt.Errorf("scanning event record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing get event record: %w", err)
	}
	return record, nil
}

func (s *EpisodicStore) ListEventLinks(ctx context.Context, tenantID, eventID string) ([]models.EventLink, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing event links: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `SELECT event_id, node_id, role, created_at
		FROM kg_event_links
		WHERE tenant_id = current_setting('app.tenant_id')::uuid AND event_id = $1
		ORDER BY created_at, node_id, role`, eventID)
	if err != nil {
		return nil, fmt.Errorf("querying event links: %w", err)
	}
	defer rows.Close()

	var links []models.EventLink
	for rows.Next() {
		var link models.EventLink
		if err := rows.Scan(&link.EventID, &link.NodeID, &link.Role, &link.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning event link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating event links: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing list event links: %w", err)
	}
	return links, nil
}

func scanEpisode(scan func(dest ...any) error) (*models.Episode, error) {
	var episode models.Episode
	var props []byte
	if err := scan(
		&episode.ID,
		&episode.TenantID,
		&episode.Title,
		&episode.Summary,
		&episode.Status,
		&episode.StartedAt,
		&episode.EndedAt,
		&episode.PrimaryProjectNodeID,
		&episode.SourceArtifactNodeID,
		&props,
		&episode.CreatedAt,
		&episode.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(props) > 0 {
		if err := json.Unmarshal(props, &episode.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling episode properties: %w", err)
		}
	}
	if episode.Properties == nil {
		episode.Properties = map[string]any{}
	}
	return &episode, nil
}

func scanEventRecord(scan func(dest ...any) error) (*models.EventRecord, error) {
	var record models.EventRecord
	var evidence []byte
	var props []byte
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
		&evidence,
		&record.SourceArtifactNodeID,
		&props,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &record.Evidence); err != nil {
			return nil, fmt.Errorf("unmarshalling event evidence: %w", err)
		}
	}
	if len(props) > 0 {
		if err := json.Unmarshal(props, &record.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling event properties: %w", err)
		}
	}
	if record.Evidence == nil {
		record.Evidence = []models.EventEvidence{}
	}
	if record.Properties == nil {
		record.Properties = map[string]any{}
	}
	return &record, nil
}
