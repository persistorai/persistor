-- +goose NO TRANSACTION
--
-- Retrieval-path indexes. Marked NO TRANSACTION so the index builds can use
-- CREATE INDEX CONCURRENTLY: unlike 001 (which created its indexes on
-- guaranteed-empty tables inside goose's transaction), this migration runs
-- against a populated notes table, so a non-concurrent build would take a
-- write-blocking lock for the duration. CONCURRENTLY cannot run inside a
-- transaction, hence the annotation above.

-- +goose Up
-- Core/brief hot path: CoreNotes filters (tenant_id, tier='core',
-- superseded=FALSE) ORDER BY id on every working-set assembly. A partial index
-- on exactly that predicate keeps it tiny, and including id lets the index also
-- satisfy the ORDER BY without a sort.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_core
    ON notes (tenant_id, id)
    WHERE tier = 'core' AND superseded = FALSE;

-- Default retrieval always wants superseded = FALSE. A partial index over the
-- active rows is smaller and more selective than indexing the low-cardinality
-- boolean column, so it replaces idx_notes_tenant_superseded.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_active
    ON notes (tenant_id)
    WHERE superseded = FALSE;

DROP INDEX CONCURRENTLY IF EXISTS idx_notes_tenant_superseded;

-- +goose Down
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_tenant_superseded
    ON notes (tenant_id, superseded);

DROP INDEX CONCURRENTLY IF EXISTS idx_notes_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_core;
