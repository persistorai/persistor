-- +goose NO TRANSACTION
-- +goose Up
-- Add trailing created_at DESC to audit indexes so queries that filter by entity or
-- action and order by recency can be satisfied by the index without a separate sort step.
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_entity;
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_action;
CREATE INDEX CONCURRENTLY idx_audit_entity ON kg_audit_log(tenant_id, entity_type, entity_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_audit_action ON kg_audit_log(tenant_id, action, created_at DESC);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_entity;
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_action;
CREATE INDEX CONCURRENTLY idx_audit_entity ON kg_audit_log(tenant_id, entity_type, entity_id);
CREATE INDEX CONCURRENTLY idx_audit_action ON kg_audit_log(tenant_id, action);
