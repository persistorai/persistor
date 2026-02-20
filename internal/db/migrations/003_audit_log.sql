-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE kg_audit_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    actor       TEXT,
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- RLS
ALTER TABLE kg_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kg_audit_log
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Indexes for common queries
CREATE INDEX CONCURRENTLY idx_audit_tenant_created ON kg_audit_log(tenant_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_audit_entity ON kg_audit_log(tenant_id, entity_type, entity_id);
CREATE INDEX CONCURRENTLY idx_audit_action ON kg_audit_log(tenant_id, action);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_action;
DROP INDEX IF EXISTS idx_audit_entity;
DROP INDEX IF EXISTS idx_audit_tenant_created;
DROP POLICY IF EXISTS tenant_isolation ON kg_audit_log;
DROP TABLE IF EXISTS kg_audit_log;
