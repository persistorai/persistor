package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// AuditStore provides data access for the kg_audit_log table.
type AuditStore struct {
	Base
}

// NewAuditStore creates an AuditStore.
func NewAuditStore(base Base) *AuditStore {
	return &AuditStore{Base: base}
}

// RecordAudit inserts an audit log entry.
func (s *AuditStore) RecordAudit(
	ctx context.Context,
	tenantID, action, entityType, entityID, actor string,
	detail map[string]any,
) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on early return.

	var detailJSON []byte
	if detail != nil {
		detailJSON, err = json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshaling audit detail: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO kg_audit_log (tenant_id, action, entity_type, entity_id, actor, detail)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, action, entityType, entityID, actor, detailJSON,
	)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}

	return tx.Commit(ctx)
}

// buildAuditFilter builds WHERE clause and args from AuditQueryOpts.
func buildAuditFilter(opts models.AuditQueryOpts) (where string, args []any, nextArg int) {
	var conditions []string
	argIdx := 1

	if opts.EntityType != "" {
		conditions = append(conditions, "entity_type = $"+strconv.Itoa(argIdx))
		args = append(args, opts.EntityType)
		argIdx++
	}
	if opts.EntityID != "" {
		conditions = append(conditions, "entity_id = $"+strconv.Itoa(argIdx))
		args = append(args, opts.EntityID)
		argIdx++
	}
	if opts.Action != "" {
		conditions = append(conditions, "action = $"+strconv.Itoa(argIdx))
		args = append(args, opts.Action)
		argIdx++
	}
	if opts.Since != nil {
		conditions = append(conditions, "created_at >= $"+strconv.Itoa(argIdx))
		args = append(args, *opts.Since)
		argIdx++
	}

	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	return where, args, argIdx
}

// QueryAudit returns audit entries matching the given filters.
// Returns entries, hasMore flag, and any error.
func (s *AuditStore) QueryAudit(
	ctx context.Context, tenantID string, opts models.AuditQueryOpts,
) ([]models.AuditEntry, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginReadTx(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on early return.

	where, args, argIdx := buildAuditFilter(opts)

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(
		"SELECT id, tenant_id, action, entity_type, entity_id, actor, detail, created_at FROM kg_audit_log %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, argIdx, argIdx+1,
	)
	args = append(args, limit+1, opts.Offset)

	entries, err := scanAuditRows(ctx, tx, query, args, s.Log)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	return entries, hasMore, nil
}

// scanAuditRows executes a query and scans audit entries from the result.
func scanAuditRows(ctx context.Context, tx pgx.Tx, query string, args []any, log *logrus.Logger) ([]models.AuditEntry, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit log: %w", err)
	}
	defer rows.Close()

	var entries []models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var detailJSON []byte
		var actor *string

		if err := rows.Scan(&e.ID, &e.TenantID, &e.Action, &e.EntityType, &e.EntityID, &actor, &detailJSON, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		if actor != nil {
			e.Actor = *actor
		}
		if detailJSON != nil {
			if err := json.Unmarshal(detailJSON, &e.Detail); err != nil {
				log.WithError(err).Warn("failed to unmarshal audit detail")
			}
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// purgeBatchSize limits the number of rows deleted per transaction to avoid
// holding long locks on kg_audit_log.
const purgeBatchSize = 5000

// PurgeOldEntries deletes audit entries older than retentionDays in batches.
// Returns the number of deleted entries.
func (s *AuditStore) PurgeOldEntries(
	ctx context.Context, tenantID string, retentionDays int,
) (int, error) {
	var totalDeleted int

	for {
		batchCtx, cancel := withTimeout(ctx)

		deleted, err := s.purgeOldEntriesBatch(batchCtx, tenantID, retentionDays)
		cancel()

		if err != nil {
			return totalDeleted, err
		}

		totalDeleted += deleted
		if deleted < purgeBatchSize {
			break
		}
	}

	return totalDeleted, nil
}

// purgeOldEntriesBatch deletes a single batch of expired audit entries.
func (s *AuditStore) purgeOldEntriesBatch(
	ctx context.Context, tenantID string, retentionDays int,
) (int, error) {
	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on early return.

	tag, err := tx.Exec(ctx,
		`DELETE FROM kg_audit_log WHERE ctid IN (
			SELECT ctid FROM kg_audit_log
			WHERE created_at < NOW() - make_interval(days => $1)
			LIMIT $2
		)`,
		retentionDays, purgeBatchSize,
	)
	if err != nil {
		return 0, fmt.Errorf("purging audit entries: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return int(tag.RowsAffected()), nil
}
