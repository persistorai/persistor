package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/persistorai/persistor/internal/models"
)

// EdgeStore provides edge CRUD operations.
type EdgeStore struct {
	Base
}

// NewEdgeStore creates a new EdgeStore.
func NewEdgeStore(base Base) *EdgeStore {
	return &EdgeStore{Base: base}
}

// CreateEdge inserts a new edge and returns the created record.
func (s *EdgeStore) CreateEdge(
	ctx context.Context,
	tenantID string,
	req models.CreateEdgeRequest,
) (*models.Edge, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating edge: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Verify source and target nodes exist in a single query.
	var sourceExists, targetExists bool
	err = tx.QueryRow(ctx,
		`SELECT
			EXISTS(SELECT 1 FROM kg_nodes WHERE tenant_id = $1 AND id = $2),
			EXISTS(SELECT 1 FROM kg_nodes WHERE tenant_id = $1 AND id = $3)`,
		tenantID, req.Source, req.Target).Scan(&sourceExists, &targetExists)
	if err != nil {
		return nil, fmt.Errorf("checking source/target nodes: %w", err)
	}

	if !sourceExists {
		return nil, fmt.Errorf("source node %q: %w", req.Source, models.ErrNodeNotFound)
	}

	if !targetExists {
		return nil, fmt.Errorf("target node %q: %w", req.Target, models.ErrNodeNotFound)
	}

	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := s.encryptProperties(ctx, tenantID, props)
	if err != nil {
		return nil, fmt.Errorf("preparing edge properties: %w", err)
	}

	weight := 1.0
	if req.Weight != nil {
		weight = *req.Weight
	}

	dateLower, dateUpper, dateQualifier, err := parseTemporalBounds(req.DateStart, req.DateEnd)
	if err != nil {
		return nil, fmt.Errorf("parsing temporal bounds: %w", err)
	}

	query := `INSERT INTO kg_edges
		(tenant_id, source, target, relation, properties, weight,
		 date_start, date_end, date_lower, date_upper, is_current, date_qualifier)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING ` + edgeColumns

	row := tx.QueryRow(ctx, query,
		tenantID, req.Source, req.Target, req.Relation, propsJSON, weight,
		req.DateStart, req.DateEnd, dateLower, dateUpper, req.IsCurrent, dateQualifier,
	)

	e, err := scanEdge(row.Scan)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}

		return nil, fmt.Errorf("scanning created edge: %w", err)
	}

	if err := s.decryptEdge(ctx, tenantID, e); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing create edge: %w", err)
	}

	s.notify("kg_edges", "insert", tenantID)

	return e, nil
}

// UpdateEdge updates an existing edge by composite key and returns the result.
func (s *EdgeStore) UpdateEdge(
	ctx context.Context,
	tenantID string,
	source, target, relation string,
	req models.UpdateEdgeRequest,
) (*models.Edge, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("updating edge: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	setClauses, args, argIdx, err := s.buildEdgeUpdateClauses(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	if len(setClauses) == 0 {
		e, err := s.getEdge(ctx, tx, source, target, relation)
		if err != nil {
			return nil, err
		}

		if err := s.decryptEdge(ctx, tenantID, e); err != nil {
			return nil, err
		}

		return e, nil
	}

	query := fmt.Sprintf(
		"UPDATE kg_edges SET %s WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $%d AND target = $%d AND relation = $%d RETURNING %s",
		strings.Join(setClauses, ", "),
		argIdx,
		argIdx+1,
		argIdx+2,
		edgeColumns,
	)
	args = append(args, source, target, relation)

	row := tx.QueryRow(ctx, query, args...)

	e, err := scanEdge(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrEdgeNotFound
		}

		return nil, fmt.Errorf("scanning updated edge: %w", err)
	}

	if err := s.decryptEdge(ctx, tenantID, e); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing update edge: %w", err)
	}

	s.notify("kg_edges", "update", tenantID)

	return e, nil
}

// buildEdgeUpdateClauses builds the SET clause list and args for UpdateEdge.
func (s *EdgeStore) buildEdgeUpdateClauses(
	ctx context.Context,
	tenantID string,
	req models.UpdateEdgeRequest,
) (setClauses []string, args []any, argIdx int, err error) {
	setClauses = make([]string, 0, 8)
	args = make([]any, 0, 10)
	argIdx = 1

	if req.Properties != nil {
		propsJSON, encErr := s.encryptProperties(ctx, tenantID, req.Properties)
		if encErr != nil {
			return nil, nil, 0, fmt.Errorf("preparing edge properties: %w", encErr)
		}

		setClauses = append(setClauses, fmt.Sprintf("properties = $%d", argIdx))
		args = append(args, propsJSON)
		argIdx++
	}

	if req.Weight != nil {
		setClauses = append(setClauses, fmt.Sprintf("weight = $%d", argIdx))
		args = append(args, *req.Weight)
		argIdx++
	}

	if req.DateStart != nil || req.DateEnd != nil {
		dateLower, dateUpper, dateQualifier, parseErr := parseTemporalBounds(req.DateStart, req.DateEnd)
		if parseErr != nil {
			return nil, nil, 0, fmt.Errorf("parsing temporal bounds: %w", parseErr)
		}

		setClauses = append(setClauses,
			fmt.Sprintf("date_start = $%d", argIdx),
			fmt.Sprintf("date_end = $%d", argIdx+1),
			fmt.Sprintf("date_lower = $%d", argIdx+2),
			fmt.Sprintf("date_upper = $%d", argIdx+3),
			fmt.Sprintf("date_qualifier = $%d", argIdx+4),
		)
		args = append(args, req.DateStart, req.DateEnd, dateLower, dateUpper, dateQualifier)
		argIdx += 5
	}

	if req.IsCurrent != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_current = $%d", argIdx))
		args = append(args, *req.IsCurrent)
		argIdx++
	}

	return setClauses, args, argIdx, nil
}
