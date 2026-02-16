package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// node represents a knowledge graph node read from SQLite.
type node struct {
	ID            string
	Type          string
	Label         string
	Properties    sql.NullString
	Created       string
	Updated       string
	AccessCount   int
	LastAccessed  sql.NullString
	SalienceScore float64
	SupersededBy  sql.NullString
	UserBoosted   int
}

// readNodes reads all kg_nodes from SQLite.
func readNodes(ctx context.Context, db *sql.DB) ([]node, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, type, label, properties, created, updated,
		        access_count, last_accessed, salience_score, superseded_by, user_boosted
		 FROM kg_nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []node
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.ID, &n.Type, &n.Label, &n.Properties,
			&n.Created, &n.Updated, &n.AccessCount, &n.LastAccessed,
			&n.SalienceScore, &n.SupersededBy, &n.UserBoosted); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// insertNodes batch-inserts nodes into PostgreSQL in groups of 100.
func insertNodes(ctx context.Context, tx pgx.Tx, nodes []node, tenantID string, enc *encryptor) error {
	const batchSize = 100
	for i := 0; i < len(nodes); i += batchSize {
		end := min(i+batchSize, len(nodes))
		if err := insertNodeBatch(ctx, tx, nodes[i:end], tenantID, enc); err != nil {
			return fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
	}
	return nil
}

// insertNodeBatch inserts a single batch of nodes.
func insertNodeBatch(ctx context.Context, tx pgx.Tx, batch []node, tenantID string, enc *encryptor) error {
	for i := range batch {
		n := &batch[i]
		createdAt := parseTime(n.Created)
		updatedAt := parseTime(n.Updated)
		props, err := encryptProps(enc, normalizeJSON(n.Properties), tenantID)
		if err != nil {
			return fmt.Errorf("encrypting node %s properties: %w", n.ID, err)
		}
		lastAccessed := parseNullableTime(n.LastAccessed)
		supersededBy := nullStr(n.SupersededBy)

		_, err = tx.Exec(ctx,
			`INSERT INTO kg_nodes (id, tenant_id, type, label, properties,
			    access_count, last_accessed, salience_score, superseded_by, user_boosted,
			    created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			 ON CONFLICT (tenant_id, id) DO NOTHING`,
			n.ID, tenantID, n.Type, n.Label, props,
			n.AccessCount, lastAccessed, n.SalienceScore, supersededBy, n.UserBoosted != 0,
			createdAt, updatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert node %s: %w", n.ID, err)
		}
	}
	return nil
}

// buildNodeSet creates a set of node IDs for fast lookup.
func buildNodeSet(nodes []node) map[string]bool {
	m := make(map[string]bool, len(nodes))
	for i := range nodes {
		m[nodes[i].ID] = true
	}
	return m
}
