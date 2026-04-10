package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/persistorai/persistor/internal/models"
)

const aliasColumns = `id, tenant_id, node_id, alias, normalized_alias, alias_type, confidence, source, created_at`

// AliasStore provides persisted alias CRUD operations.
type AliasStore struct {
	Base
}

// NewAliasStore creates a new AliasStore.
func NewAliasStore(base Base) *AliasStore {
	return &AliasStore{Base: base}
}

// CreateAlias inserts a new alias record and returns it.
func (s *AliasStore) CreateAlias(ctx context.Context, tenantID string, req models.CreateAliasRequest) (*models.Alias, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating alias: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	normalized := models.NormalizeAlias(req.Alias)
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO kg_aliases (tenant_id, node_id, alias, normalized_alias, alias_type, confidence, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+aliasColumns,
		tenantID, req.NodeID, req.Alias, normalized, req.AliasType, confidence, req.Source,
	)

	a, err := scanAlias(row.Scan)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}
		return nil, fmt.Errorf("scanning created alias: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing create alias: %w", err)
	}

	s.notify("kg_aliases", "insert", tenantID)
	return a, nil
}

// GetAlias returns a single alias by ID.
func (s *AliasStore) GetAlias(ctx context.Context, tenantID, aliasID string) (*models.Alias, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting alias: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `SELECT `+aliasColumns+` FROM kg_aliases WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`, aliasID)
	a, err := scanAlias(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrAliasNotFound
		}
		return nil, fmt.Errorf("scanning alias: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing get alias: %w", err)
	}

	return a, nil
}

// ListAliases returns aliases for a tenant with optional filters.
func (s *AliasStore) ListAliases(ctx context.Context, tenantID string, opts models.AliasListOpts) ([]models.Alias, bool, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > maxListLimit {
		opts.Limit = maxListLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, false, fmt.Errorf("listing aliases: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	where := " WHERE tenant_id = current_setting('app.tenant_id')::uuid"
	args := make([]any, 0, 5)
	argIdx := 1

	if opts.NodeID != "" {
		where += fmt.Sprintf(" AND node_id = $%d", argIdx)
		args = append(args, opts.NodeID)
		argIdx++
	}
	if opts.NormalizedAlias != "" {
		where += fmt.Sprintf(" AND normalized_alias = $%d", argIdx)
		args = append(args, models.NormalizeAlias(opts.NormalizedAlias))
		argIdx++
	}
	if opts.AliasType != "" {
		where += fmt.Sprintf(" AND alias_type = $%d", argIdx)
		args = append(args, opts.AliasType)
		argIdx++
	}

	query := `SELECT ` + aliasColumns + ` FROM kg_aliases` + where + ` ORDER BY normalized_alias, created_at ASC`
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, opts.Limit+1, opts.Offset)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("querying aliases: %w", err)
	}
	defer rows.Close()

	aliases, err := collectAliases(rows)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(aliases) > opts.Limit
	if hasMore {
		aliases = aliases[:opts.Limit]
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing list aliases: %w", err)
	}

	return aliases, hasMore, nil
}

// DeleteAlias removes an alias by ID.
func (s *AliasStore) DeleteAlias(ctx context.Context, tenantID, aliasID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("deleting alias: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `DELETE FROM kg_aliases WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1`, aliasID)
	if err != nil {
		return fmt.Errorf("executing delete alias: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrAliasNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing delete alias: %w", err)
	}

	s.notify("kg_aliases", "delete", tenantID)
	return nil
}
