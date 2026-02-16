package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

// maxBulkBatchSize limits the number of rows per INSERT statement to avoid
// exceeding PostgreSQL's parameter limit (65535 params).
const maxBulkBatchSize = 500

// BulkStore handles bulk upsert operations for nodes and edges.
type BulkStore struct {
	Base
}

// NewBulkStore creates a BulkStore with the given shared base.
func NewBulkStore(base Base) *BulkStore {
	return &BulkStore{Base: base}
}

// BulkUpsertNodes inserts or updates multiple nodes in a single transaction
// using multi-row INSERT ... ON CONFLICT. Returns the number of upserted rows.
func (s *BulkStore) BulkUpsertNodes( //nolint:gocognit,gocyclo,cyclop,funlen // complexity from batch building + history tracking.
	ctx context.Context,
	tenantID string,
	nodes []models.CreateNodeRequest,
) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// Pre-encrypt all properties BEFORE opening the transaction to minimize lock time.
	encryptedProps := make([][]byte, len(nodes))
	for i, node := range nodes {
		props := node.Properties
		if props == nil {
			props = map[string]any{}
		}

		propsJSON, err := s.encryptProperties(ctx, tenantID, props)
		if err != nil {
			return 0, fmt.Errorf("preparing node %s properties: %w", node.ID, err)
		}

		encryptedProps[i] = propsJSON
	}

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert nodes: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Fetch existing node properties for history tracking.
	existingNodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		existingNodeIDs[i] = n.ID
	}

	oldPropsMap, err := s.fetchExistingProperties(ctx, tx, tenantID, existingNodeIDs)
	if err != nil {
		return 0, fmt.Errorf("fetching existing properties for history: %w", err)
	}

	total := 0

	// Process in batches to stay within parameter limits.
	for i := 0; i < len(nodes); i += maxBulkBatchSize {
		end := i + maxBulkBatchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		batchProps := encryptedProps[i:end]

		valueParts := make([]string, 0, len(batch))
		args := make([]any, 0, len(batch)*5)

		for j, node := range batch {
			base := j*5 + 1
			valueParts = append(valueParts, fmt.Sprintf(
				"($%d, $%d, $%d, $%d, $%d)",
				base, base+1, base+2, base+3, base+4,
			))
			args = append(args, node.ID, tenantID, node.Type, node.Label, batchProps[j])
		}

		sql := `INSERT INTO kg_nodes (id, tenant_id, type, label, properties)
			VALUES ` + strings.Join(valueParts, ", ") + `
			ON CONFLICT (tenant_id, id) DO UPDATE
			SET type = EXCLUDED.type,
				label = EXCLUDED.label,
				properties = EXCLUDED.properties,
				updated_at = NOW()`

		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return 0, fmt.Errorf("bulk upserting nodes batch: %w", err)
		}

		total += int(tag.RowsAffected())
	}

	// Record property history for nodes that existed before the upsert.
	for _, node := range nodes {
		oldProps, existed := oldPropsMap[node.ID]
		if !existed {
			continue
		}

		newProps := node.Properties
		if newProps == nil {
			newProps = map[string]any{}
		}

		if err := RecordPropertyChanges(ctx, tx, tenantID, node.ID, oldProps, newProps, "bulk_upsert"); err != nil {
			return 0, fmt.Errorf("recording property history for %s: %w", node.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing bulk upsert nodes: %w", err)
	}

	// Send aggregate notification (best-effort) using a fresh context.
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer notifyCancel()

	payload, _ := json.Marshal(map[string]any{ //nolint:errcheck // static keys, cannot fail.
		"table":     "kg_nodes",
		"op":        "BULK",
		"count":     total,
		"tenant_id": tenantID,
	})

	if _, err := s.Pool.Exec(notifyCtx, "SELECT pg_notify('kg_changes', $1)", string(payload)); err != nil {
		s.Log.WithError(err).Warn("failed to send bulk node notification")
	}

	return total, nil
}

// BulkUpsertEdges inserts or updates multiple edges in a single transaction
// using multi-row INSERT ... ON CONFLICT. Returns the number of upserted rows.
func (s *BulkStore) BulkUpsertEdges( //nolint:gocognit,gocyclo,cyclop,funlen // complexity from batch building + node existence validation.
	ctx context.Context,
	tenantID string,
	edges []models.CreateEdgeRequest,
) (int, error) {
	if len(edges) == 0 {
		return 0, nil
	}

	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// Pre-encrypt all properties BEFORE opening the transaction to minimize lock time.
	encryptedProps := make([][]byte, len(edges))
	for i, edge := range edges {
		props := edge.Properties
		if props == nil {
			props = map[string]any{}
		}

		propsJSON, err := s.encryptProperties(ctx, tenantID, props)
		if err != nil {
			return 0, fmt.Errorf("preparing edge %s->%s properties: %w", edge.Source, edge.Target, err)
		}

		encryptedProps[i] = propsJSON
	}

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert edges: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Verify all referenced nodes exist.
	nodeIDSet := make(map[string]struct{})
	for _, edge := range edges {
		nodeIDSet[edge.Source] = struct{}{}
		nodeIDSet[edge.Target] = struct{}{}
	}

	expectedIDs := make([]string, 0, len(nodeIDSet))
	for id := range nodeIDSet {
		expectedIDs = append(expectedIDs, id)
	}

	rows, err := tx.Query(ctx,
		`SELECT id FROM kg_nodes WHERE tenant_id = $1 AND id = ANY($2)`,
		tenantID, expectedIDs)
	if err != nil {
		return 0, fmt.Errorf("verifying node existence: %w", err)
	}

	foundIDs := make(map[string]struct{})

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning node ID: %w", err)
		}

		foundIDs[id] = struct{}{}
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating node IDs: %w", err)
	}

	if len(foundIDs) != len(nodeIDSet) {
		var missing []string
		for id := range nodeIDSet {
			if _, ok := foundIDs[id]; !ok {
				missing = append(missing, id)
			}
		}

		return 0, fmt.Errorf("missing node IDs referenced by edges: %v", missing)
	}

	total := 0

	for i := 0; i < len(edges); i += maxBulkBatchSize {
		end := i + maxBulkBatchSize
		if end > len(edges) {
			end = len(edges)
		}

		batch := edges[i:end]
		batchProps := encryptedProps[i:end]

		valueParts := make([]string, 0, len(batch))
		args := make([]any, 0, len(batch)*6)

		for j, edge := range batch {
			weight := 1.0
			if edge.Weight != nil {
				weight = *edge.Weight
			}

			base := j*6 + 1
			valueParts = append(valueParts, fmt.Sprintf(
				"($%d, $%d, $%d, $%d, $%d, $%d)",
				base, base+1, base+2, base+3, base+4, base+5,
			))
			args = append(args, tenantID, edge.Source, edge.Target, edge.Relation, batchProps[j], weight)
		}

		sql := `INSERT INTO kg_edges (tenant_id, source, target, relation, properties, weight)
			VALUES ` + strings.Join(valueParts, ", ") + `
			ON CONFLICT (tenant_id, source, target, relation) DO UPDATE
			SET properties = EXCLUDED.properties,
				weight = EXCLUDED.weight,
				updated_at = NOW()`

		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return 0, fmt.Errorf("bulk upserting edges batch: %w", err)
		}

		total += int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing bulk upsert edges: %w", err)
	}

	// Send aggregate notification (best-effort) using a fresh context.
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer notifyCancel()

	payload, _ := json.Marshal(map[string]any{ //nolint:errcheck // static keys, cannot fail.
		"table":     "kg_edges",
		"op":        "BULK",
		"count":     total,
		"tenant_id": tenantID,
	})

	if _, err := s.Pool.Exec(notifyCtx, "SELECT pg_notify('kg_changes', $1)", string(payload)); err != nil {
		s.Log.WithError(err).Warn("failed to send bulk edge notification")
	}

	return total, nil
}
