package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// edge represents a knowledge graph edge read from SQLite.
type edge struct {
	Source        string
	Target        string
	Relation      string
	Properties    sql.NullString
	Created       string
	Updated       string
	Weight        float64
	AccessCount   int
	LastAccessed  sql.NullString
	SalienceScore float64
	SupersededBy  sql.NullString
	UserBoosted   int
}

// readEdges reads all kg_edges from SQLite.
func readEdges(ctx context.Context, db *sql.DB) ([]edge, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT source, target, relation, properties, created, updated,
		        weight, access_count, last_accessed, salience_score, superseded_by, user_boosted
		 FROM kg_edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.Source, &e.Target, &e.Relation, &e.Properties,
			&e.Created, &e.Updated, &e.Weight, &e.AccessCount, &e.LastAccessed,
			&e.SalienceScore, &e.SupersededBy, &e.UserBoosted); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// insertEdges batch-inserts edges, skipping those with missing source/target nodes.
func insertEdges(ctx context.Context, tx pgx.Tx, edges []edge, tenantID string, nodeIDs map[string]bool, enc *encryptor) (int, []skippedEdge) {
	var skipped []skippedEdge
	inserted := 0

	for i := 0; i < len(edges); i++ {
		e := edges[i]
		if !nodeIDs[e.Source] {
			skipped = append(skipped, skippedEdge{e.Source, e.Target, "source node not found"})
			continue
		}
		if !nodeIDs[e.Target] {
			skipped = append(skipped, skippedEdge{e.Source, e.Target, "target node not found"})
			continue
		}

		createdAt := parseTime(e.Created)
		updatedAt := parseTime(e.Updated)
		props, encErr := encryptProps(enc, normalizeJSON(e.Properties), tenantID)
		if encErr != nil {
			slog.Warn("edge property encryption failed, skipping", "source", e.Source, "target", e.Target, "error", encErr)
			skipped = append(skipped, skippedEdge{e.Source, e.Target, encErr.Error()})
			continue
		}
		lastAccessed := parseNullableTime(e.LastAccessed)
		supersededBy := nullStr(e.SupersededBy)

		_, err := tx.Exec(ctx,
			`INSERT INTO kg_edges (tenant_id, source, target, relation, properties, weight,
			    access_count, last_accessed, salience_score, superseded_by, user_boosted,
			    created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			 ON CONFLICT (tenant_id, source, target, relation) DO NOTHING`,
			tenantID, e.Source, e.Target, e.Relation, props, e.Weight,
			e.AccessCount, lastAccessed, e.SalienceScore, supersededBy, e.UserBoosted != 0,
			createdAt, updatedAt,
		)
		if err != nil {
			slog.Warn("edge insert failed, skipping", "source", e.Source, "target", e.Target, "error", err)
			skipped = append(skipped, skippedEdge{e.Source, e.Target, err.Error()})
			continue
		}
		inserted++
	}
	return inserted, skipped
}
