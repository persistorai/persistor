// Package db provides database migration and maintenance utilities.
package db

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
)

// EnsureVectorDimensions checks that the kg_nodes.embedding column matches the
// configured dimensions and alters it (with index rebuild) if not.
// This allows operators to change EMBEDDING_DIMENSIONS and have the schema
// adapt on next restart. Existing embeddings with mismatched dimensions will
// be set to NULL so they can be re-generated.
func EnsureVectorDimensions(ctx context.Context, pool *dbpool.Pool, log *logrus.Logger, dimensions int) error {
	if dimensions < 1 || dimensions > 4096 {
		return fmt.Errorf("embedding dimensions must be between 1 and 4096, got %d", dimensions)
	}

	// Query current column type from information_schema via pg_attribute + format_type.
	var currentType string
	err := pool.QueryRow(ctx,
		`SELECT format_type(a.atttypid, a.atttypmod)
		 FROM pg_attribute a
		 JOIN pg_class c ON c.oid = a.attrelid
		 WHERE c.relname = 'kg_nodes' AND a.attname = 'embedding' AND NOT a.attisdropped`,
	).Scan(&currentType)
	if err != nil {
		return fmt.Errorf("querying embedding column type: %w", err)
	}

	expectedType := fmt.Sprintf("vector(%d)", dimensions)
	if currentType == expectedType {
		log.WithField("dimensions", dimensions).Debug("embedding column dimensions match config")
		return nil
	}

	log.WithFields(logrus.Fields{
		"current":  currentType,
		"expected": expectedType,
	}).Info("embedding column dimensions changed, altering schema")

	// Drop the HNSW index, alter column, null out mismatched embeddings, rebuild index.
	// This runs in a transaction for safety.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning dimension alter tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback after commit.

	// Drop existing HNSW index.
	if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS idx_nodes_embedding`); err != nil {
		return fmt.Errorf("dropping embedding index: %w", err)
	}

	// Null out embeddings that don't match new dimensions (they need re-generation).
	if _, err := tx.Exec(ctx,
		`UPDATE kg_nodes SET embedding = NULL WHERE embedding IS NOT NULL AND vector_dims(embedding) != $1`,
		dimensions,
	); err != nil {
		return fmt.Errorf("nulling mismatched embeddings: %w", err)
	}

	// Alter column type.
	alterSQL := fmt.Sprintf(`ALTER TABLE kg_nodes ALTER COLUMN embedding TYPE vector(%d)`, dimensions)
	if _, err := tx.Exec(ctx, alterSQL); err != nil {
		return fmt.Errorf("altering embedding column: %w", err)
	}

	// Recreate HNSW index.
	if _, err := tx.Exec(ctx,
		`CREATE INDEX idx_nodes_embedding ON kg_nodes USING hnsw (embedding vector_cosine_ops)
		 WITH (m = 32, ef_construction = 200) WHERE embedding IS NOT NULL`,
	); err != nil {
		return fmt.Errorf("recreating embedding index: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing dimension alter: %w", err)
	}

	log.WithField("dimensions", dimensions).Info("embedding column dimensions updated")
	return nil
}
