package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/models"
)

// nodeColumns lists the columns selected for node queries (excluding embedding).
const nodeColumns = `id, tenant_id, type, label, properties,
	access_count, last_accessed, salience_score, superseded_by,
	user_boosted, created_at, updated_at`

// edgeColumns lists the columns selected for edge queries.
const edgeColumns = `tenant_id, source, target, relation, properties,
	weight, access_count, last_accessed, salience_score, superseded_by,
	user_boosted, created_at, updated_at`

// scanNode scans a single row into a models.Node.
func scanNode(scan func(dest ...any) error) (*models.Node, error) {
	var n models.Node
	var tenantID uuid.UUID
	var props []byte
	var lastAccessed *time.Time
	var supersededBy *string

	err := scan(
		&n.ID,
		&tenantID,
		&n.Type,
		&n.Label,
		&props,
		&n.AccessCount,
		&lastAccessed,
		&n.Salience,
		&supersededBy,
		&n.UserBoosted,
		&n.CreatedAt,
		&n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	n.TenantID = tenantID
	n.LastAccessed = lastAccessed
	n.SupersededBy = supersededBy

	if err := json.Unmarshal(props, &n.Properties); err != nil {
		return nil, fmt.Errorf("unmarshalling node properties: %w", err)
	}

	return &n, nil
}

// scanEdge scans a single row into a models.Edge.
func scanEdge(scan func(dest ...any) error) (*models.Edge, error) {
	var e models.Edge
	var tenantID uuid.UUID
	var props []byte
	var lastAccessed *time.Time
	var supersededBy *string

	err := scan(
		&tenantID,
		&e.Source,
		&e.Target,
		&e.Relation,
		&props,
		&e.Weight,
		&e.AccessCount,
		&lastAccessed,
		&e.Salience,
		&supersededBy,
		&e.UserBoosted,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	e.TenantID = tenantID
	e.LastAccessed = lastAccessed
	e.SupersededBy = supersededBy

	if err := json.Unmarshal(props, &e.Properties); err != nil {
		return nil, fmt.Errorf("unmarshalling edge properties: %w", err)
	}

	return &e, nil
}

// collectEdges scans all rows into an edge slice.
func collectEdges(rows pgx.Rows) ([]models.Edge, error) {
	edges := make([]models.Edge, 0, 16)

	for rows.Next() {
		e, err := scanEdge(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning edge row: %w", err)
		}

		edges = append(edges, *e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating edge rows: %w", err)
	}

	return edges, nil
}

// collectNodes scans all rows into a node slice.
func collectNodes(rows pgx.Rows) ([]models.Node, error) {
	nodes := make([]models.Node, 0, 16)

	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}

		nodes = append(nodes, *n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating node rows: %w", err)
	}

	return nodes, nil
}
