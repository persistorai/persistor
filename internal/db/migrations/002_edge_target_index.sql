-- +goose NO TRANSACTION
-- +goose Up
-- Add composite index to support ListEdges queries filtered by target ordered by recency.
-- The existing idx_edges_tenant_target index lacks updated_at, causing a sort step on
-- every target-filtered list query. This index covers the (tenant_id, target, updated_at DESC)
-- access pattern used by buildEdgeListQuery in internal/store/edge_read.go.
CREATE INDEX CONCURRENTLY idx_edges_tenant_target_updated ON kg_edges(tenant_id, target, updated_at DESC);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_edges_tenant_target_updated;
