-- +goose Up
-- Fix: kg_property_history was missing FORCE ROW LEVEL SECURITY and WITH CHECK.
-- The table owner could bypass RLS entirely. Drop the old policy and recreate
-- with both USING and WITH CHECK to match kg_nodes, kg_edges, kg_audit_log.

ALTER TABLE kg_property_history FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_property_history ON kg_property_history;
CREATE POLICY tenant_isolation_property_history ON kg_property_history
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation_property_history ON kg_property_history;
CREATE POLICY tenant_isolation_property_history ON kg_property_history
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

ALTER TABLE kg_property_history NO FORCE ROW LEVEL SECURITY;
