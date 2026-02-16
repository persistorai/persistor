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

// NodeStore handles node CRUD operations.
type NodeStore struct {
	Base
}

// NewNodeStore creates a new NodeStore.
func NewNodeStore(base Base) *NodeStore {
	return &NodeStore{Base: base}
}

// CreateNode inserts a new node and returns the created record.
func (s *NodeStore) CreateNode(
	ctx context.Context,
	tenantID string,
	req models.CreateNodeRequest,
) (*models.Node, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := s.encryptProperties(ctx, tenantID, props)
	if err != nil {
		return nil, fmt.Errorf("preparing node properties: %w", err)
	}

	query := `INSERT INTO kg_nodes (id, tenant_id, type, label, properties)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + nodeColumns

	row := tx.QueryRow(ctx, query, req.ID, tenantID, req.Type, req.Label, propsJSON)

	n, err := scanNode(row.Scan)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, models.ErrDuplicateKey
		}

		return nil, fmt.Errorf("scanning created node: %w", err)
	}

	if err := s.decryptNode(ctx, tenantID, n); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing create node: %w", err)
	}

	s.notify("kg_nodes", "insert", tenantID)

	return n, nil
}

// buildNodeUpdateQuery constructs the SET clause and arguments for UpdateNode.
// Returns the set clauses, query args, and the next argument index.
func (s *NodeStore) buildNodeUpdateQuery(
	ctx context.Context,
	tenantID string,
	req models.UpdateNodeRequest,
) (setClauses []string, args []any, nextArg int, err error) {
	setClauses = make([]string, 0, 3)
	args = make([]any, 0, 4)
	argIdx := 1

	if req.Type != nil {
		setClauses = append(setClauses, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *req.Type)
		argIdx++
	}

	if req.Label != nil {
		setClauses = append(setClauses, fmt.Sprintf("label = $%d", argIdx))
		args = append(args, *req.Label)
		argIdx++
	}

	if req.Properties != nil {
		propsJSON, err := s.encryptProperties(ctx, tenantID, req.Properties)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("preparing node properties: %w", err)
		}

		setClauses = append(setClauses, fmt.Sprintf("properties = $%d", argIdx))
		args = append(args, propsJSON)
		argIdx++
	}

	return setClauses, args, argIdx, nil
}

// UpdateNode updates an existing node with the provided fields and returns the result.
func (s *NodeStore) UpdateNode(
	ctx context.Context,
	tenantID string,
	nodeID string,
	req models.UpdateNodeRequest,
) (*models.Node, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	setClauses, args, argIdx, err := s.buildNodeUpdateQuery(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	if len(setClauses) == 0 {
		return s.GetNode(ctx, tenantID, nodeID)
	}

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("updating node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	var oldProps map[string]any
	if req.Properties != nil {
		oldProps, err = fetchNodeProperties(ctx, tx, tenantID, nodeID, &s.Base)
		if err != nil {
			return nil, err
		}
	}

	query := fmt.Sprintf(
		"UPDATE kg_nodes SET %s WHERE tenant_id = $%d AND id = $%d RETURNING %s",
		strings.Join(setClauses, ", "),
		argIdx,
		argIdx+1,
		nodeColumns,
	)
	args = append(args, tenantID, nodeID)

	row := tx.QueryRow(ctx, query, args...)

	n, err := scanNode(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNodeNotFound
		}

		return nil, fmt.Errorf("scanning updated node: %w", err)
	}

	if err := s.decryptNode(ctx, tenantID, n); err != nil {
		return nil, err
	}

	if req.Properties != nil {
		if err := RecordPropertyChanges(ctx, tx, tenantID, nodeID, oldProps, req.Properties, ""); err != nil {
			return nil, fmt.Errorf("recording property history: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing update node: %w", err)
	}

	s.notify("kg_nodes", "update", tenantID)

	return n, nil
}

// DeleteNode removes a node by ID and its associated edges within the same transaction.
func (s *NodeStore) DeleteNode(ctx context.Context, tenantID, nodeID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := s.beginTx(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("deleting node: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	_, err = tx.Exec(ctx, "DELETE FROM kg_edges WHERE tenant_id = current_setting('app.tenant_id')::uuid AND source = $1", nodeID)
	if err != nil {
		return fmt.Errorf("deleting source edges for node: %w", err)
	}

	_, err = tx.Exec(ctx, "DELETE FROM kg_edges WHERE tenant_id = current_setting('app.tenant_id')::uuid AND target = $1", nodeID)
	if err != nil {
		return fmt.Errorf("deleting target edges for node: %w", err)
	}

	tag, err := tx.Exec(ctx, "DELETE FROM kg_nodes WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = $1", nodeID)
	if err != nil {
		return fmt.Errorf("executing node delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrNodeNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing delete node: %w", err)
	}

	s.notify("kg_nodes", "delete", tenantID)

	return nil
}
