-- +goose Up
-- Property history tracking for knowledge graph nodes.

CREATE TABLE IF NOT EXISTS kg_property_history (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    node_id         TEXT NOT NULL,
    property_key    TEXT NOT NULL,
    old_value       JSONB,
    new_value       JSONB,
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason          TEXT,
    changed_by      TEXT
);

CREATE INDEX idx_property_history_node ON kg_property_history (tenant_id, node_id, changed_at DESC);
CREATE INDEX idx_property_history_key ON kg_property_history (tenant_id, node_id, property_key, changed_at DESC);

ALTER TABLE kg_property_history ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_property_history ON kg_property_history
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation_property_history ON kg_property_history;
DROP TABLE IF EXISTS kg_property_history;
