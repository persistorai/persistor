package store

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// RetrievalFeedbackStore persists explicit retrieval feedback events for operator review.
type RetrievalFeedbackStore struct {
	Base
}

func NewRetrievalFeedbackStore(base Base) *RetrievalFeedbackStore {
	return &RetrievalFeedbackStore{Base: base}
}

func (s *RetrievalFeedbackStore) CreateRetrievalFeedback(ctx context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
	req = req.Normalized()
	record := &models.RetrievalFeedbackRecord{
		Query:            req.Query,
		NormalizedQuery:  models.NormalizeRetrievalQuery(req.Query),
		SearchMode:       req.SearchMode,
		Intent:           req.Intent,
		InternalRerank:   req.InternalRerank,
		RerankProfile:    req.RerankProfile,
		Outcome:          req.Outcome,
		Signals:          models.DeriveRetrievalSignals(req),
		RetrievedNodeIDs: req.RetrievedNodeIDs,
		SelectedNodeIDs:  req.SelectedNodeIDs,
		ExpectedNodeIDs:  req.ExpectedNodeIDs,
		Note:             req.Note,
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating retrieval feedback: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	query := `INSERT INTO kg_retrieval_feedback (
		tenant_id, query_text, normalized_query, search_mode, intent,
		internal_rerank, internal_rerank_profile, outcome, signals,
		retrieved_node_ids, selected_node_ids, expected_node_ids, note
	) VALUES (
		current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
		$5, $6, $7, $8,
		$9, $10, $11, $12
	)
	RETURNING id, tenant_id, created_at`
	if err := tx.QueryRow(ctx, query,
		record.Query,
		record.NormalizedQuery,
		record.SearchMode,
		record.Intent,
		record.InternalRerank,
		record.RerankProfile,
		record.Outcome,
		record.Signals,
		record.RetrievedNodeIDs,
		record.SelectedNodeIDs,
		record.ExpectedNodeIDs,
		record.Note,
	).Scan(&record.ID, &record.TenantID, &record.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting retrieval feedback: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing retrieval feedback: %w", err)
	}
	return record, nil
}

func (s *RetrievalFeedbackStore) ListRetrievalFeedback(ctx context.Context, tenantID string, opts models.RetrievalFeedbackListOpts) ([]models.RetrievalFeedbackRecord, error) {
	opts = opts.Normalized()
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing retrieval feedback: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `SELECT
		id, tenant_id, query_text, normalized_query, search_mode, intent,
		internal_rerank, internal_rerank_profile, outcome, signals,
		retrieved_node_ids, selected_node_ids, expected_node_ids, note, created_at
	FROM kg_retrieval_feedback
	WHERE tenant_id = current_setting('app.tenant_id')::uuid
	ORDER BY created_at DESC, id DESC
	LIMIT $1`, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("querying retrieval feedback: %w", err)
	}
	defer rows.Close()

	items := make([]models.RetrievalFeedbackRecord, 0, opts.Limit)
	for rows.Next() {
		var item models.RetrievalFeedbackRecord
		if err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.Query,
			&item.NormalizedQuery,
			&item.SearchMode,
			&item.Intent,
			&item.InternalRerank,
			&item.RerankProfile,
			&item.Outcome,
			&item.Signals,
			&item.RetrievedNodeIDs,
			&item.SelectedNodeIDs,
			&item.ExpectedNodeIDs,
			&item.Note,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning retrieval feedback: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating retrieval feedback: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing retrieval feedback list: %w", err)
	}
	return items, nil
}
