package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/persistorai/persistor/internal/models"
)

const defaultMergeSuggestionLimit = 25

type DuplicateCandidatePair struct {
	Left              models.Node
	Right             models.Node
	SharedNames       []string
	SameLabel         bool
	LabelAliasOverlap bool
}

// ListDuplicateCandidatePairs returns likely duplicate node pairs based on shared normalized labels/aliases.
func (s *AdminStore) ListDuplicateCandidatePairs(ctx context.Context, tenantID, typeFilter string, limit int) ([]DuplicateCandidatePair, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if limit <= 0 {
		limit = defaultMergeSuggestionLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing duplicate candidate pairs: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const candidateNodeColumns = `id, tenant_id, type, label, properties,
		access_count, last_accessed, salience_score, superseded_by,
		user_boosted, created_at, updated_at`

	query := `WITH active_nodes AS (
			SELECT ` + candidateNodeColumns + `,
				LOWER(regexp_replace(BTRIM(label), '\\s+', ' ', 'g')) AS normalized_label
			FROM kg_nodes
			WHERE tenant_id = current_setting('app.tenant_id')::uuid
			  AND superseded_by IS NULL
			  AND ($1 = '' OR type = $1)
		), node_names AS (
			SELECT id AS node_id, normalized_label AS normalized_name, 'label' AS source
			FROM active_nodes
			WHERE normalized_label <> ''
			UNION ALL
			SELECT a.node_id, a.normalized_alias AS normalized_name, 'alias' AS source
			FROM kg_aliases a
			INNER JOIN active_nodes n ON n.id = a.node_id
			WHERE a.tenant_id = current_setting('app.tenant_id')::uuid
			  AND a.normalized_alias <> ''
		), shared_names AS (
			SELECT n1.node_id AS left_id,
				n2.node_id AS right_id,
				array_agg(DISTINCT n1.normalized_name ORDER BY n1.normalized_name) AS shared_names,
				bool_or(n1.source = 'label' AND n2.source = 'label') AS same_label,
				bool_or((n1.source = 'label' AND n2.source = 'alias') OR (n1.source = 'alias' AND n2.source = 'label')) AS label_alias_overlap
			FROM node_names n1
			INNER JOIN node_names n2 ON n1.normalized_name = n2.normalized_name AND n1.node_id < n2.node_id
			GROUP BY n1.node_id, n2.node_id
		)
		SELECT
			l.id, l.tenant_id, l.type, l.label, l.properties,
			l.access_count, l.last_accessed, l.salience_score, l.superseded_by,
			l.user_boosted, l.created_at, l.updated_at,
			r.id, r.tenant_id, r.type, r.label, r.properties,
			r.access_count, r.last_accessed, r.salience_score, r.superseded_by,
			r.user_boosted, r.created_at, r.updated_at,
			s.shared_names,
			s.same_label,
			s.label_alias_overlap
		FROM shared_names s
		INNER JOIN active_nodes l ON l.id = s.left_id
		INNER JOIN active_nodes r ON r.id = s.right_id
		WHERE l.type = r.type
		ORDER BY s.same_label DESC,
			cardinality(s.shared_names) DESC,
			GREATEST(l.salience_score, r.salience_score) DESC,
			LEAST(l.id, r.id) ASC
		LIMIT $2`

	rows, err := tx.Query(ctx, query, typeFilter, limit)
	if err != nil {
		return nil, fmt.Errorf("querying duplicate candidate pairs: %w", err)
	}
	defer rows.Close()

	pairs := make([]DuplicateCandidatePair, 0, limit)
	for rows.Next() {
		var (
			pair                                DuplicateCandidatePair
			leftProps, rightProps               []byte
			leftTenantID, rightTenantID         uuid.UUID
			leftLastAccessed, rightLastAccessed *time.Time
			leftSupersededBy, rightSupersededBy *string
		)
		if err := rows.Scan(
			&pair.Left.ID, &leftTenantID, &pair.Left.Type, &pair.Left.Label, &leftProps,
			&pair.Left.AccessCount, &leftLastAccessed, &pair.Left.Salience, &leftSupersededBy,
			&pair.Left.UserBoosted, &pair.Left.CreatedAt, &pair.Left.UpdatedAt,
			&pair.Right.ID, &rightTenantID, &pair.Right.Type, &pair.Right.Label, &rightProps,
			&pair.Right.AccessCount, &rightLastAccessed, &pair.Right.Salience, &rightSupersededBy,
			&pair.Right.UserBoosted, &pair.Right.CreatedAt, &pair.Right.UpdatedAt,
			&pair.SharedNames, &pair.SameLabel, &pair.LabelAliasOverlap,
		); err != nil {
			return nil, fmt.Errorf("scanning duplicate candidate pair: %w", err)
		}
		pair.Left.TenantID = leftTenantID
		pair.Left.LastAccessed = leftLastAccessed
		pair.Left.SupersededBy = leftSupersededBy
		pair.Right.TenantID = rightTenantID
		pair.Right.LastAccessed = rightLastAccessed
		pair.Right.SupersededBy = rightSupersededBy
		if err := json.Unmarshal(leftProps, &pair.Left.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling left duplicate candidate properties: %w", err)
		}
		if err := json.Unmarshal(rightProps, &pair.Right.Properties); err != nil {
			return nil, fmt.Errorf("unmarshalling right duplicate candidate properties: %w", err)
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating duplicate candidate pairs: %w", err)
	}

	for i := range pairs {
		if err := s.decryptNode(ctx, tenantID, &pairs[i].Left); err != nil {
			return nil, err
		}
		if err := s.decryptNode(ctx, tenantID, &pairs[i].Right); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing duplicate candidate pairs: %w", err)
	}

	return pairs, nil
}
