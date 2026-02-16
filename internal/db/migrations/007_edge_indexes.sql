-- +goose Up

-- Composite index for edge listing queries that filter by source and sort by updated_at
CREATE INDEX IF NOT EXISTS idx_edges_tenant_source_updated
    ON kg_edges (tenant_id, source, updated_at DESC);

-- Optimize update triggers: only fire when row data actually changes.
-- Cannot use OLD.* IS DISTINCT FROM NEW.* because kg_nodes has a generated column (label_tsv).
-- Compare the mutable columns explicitly instead.
DROP TRIGGER IF EXISTS nodes_updated ON kg_nodes;
CREATE TRIGGER nodes_updated
    BEFORE UPDATE ON kg_nodes
    FOR EACH ROW
    WHEN (
        OLD.type IS DISTINCT FROM NEW.type
        OR OLD.label IS DISTINCT FROM NEW.label
        OR OLD.properties IS DISTINCT FROM NEW.properties
        OR OLD.embedding IS DISTINCT FROM NEW.embedding
        OR OLD.salience_score IS DISTINCT FROM NEW.salience_score
        OR OLD.superseded_by IS DISTINCT FROM NEW.superseded_by
        OR OLD.user_boosted IS DISTINCT FROM NEW.user_boosted
    )
    EXECUTE FUNCTION update_timestamp();

DROP TRIGGER IF EXISTS edges_updated ON kg_edges;
CREATE TRIGGER edges_updated
    BEFORE UPDATE ON kg_edges
    FOR EACH ROW
    WHEN (OLD.* IS DISTINCT FROM NEW.*)
    EXECUTE FUNCTION update_timestamp();

-- +goose Down

-- Restore original triggers without IS DISTINCT FROM guard
DROP TRIGGER IF EXISTS nodes_updated ON kg_nodes;
CREATE TRIGGER nodes_updated
    BEFORE UPDATE ON kg_nodes
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

DROP TRIGGER IF EXISTS edges_updated ON kg_edges;
CREATE TRIGGER edges_updated
    BEFORE UPDATE ON kg_edges
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

DROP INDEX IF EXISTS idx_edges_tenant_source_updated;
