-- +goose Up
-- +goose NO TRANSACTION
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_nodes_embedding_null ON kg_nodes (tenant_id) WHERE embedding IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_nodes_embedding_null;
