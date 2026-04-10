-- +goose Up
CREATE TABLE kg_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    node_id TEXT NOT NULL CONSTRAINT chk_alias_node_id_len CHECK (length(node_id) <= 255),
    alias TEXT NOT NULL CONSTRAINT chk_alias_alias_len CHECK (length(alias) <= 1000),
    normalized_alias TEXT NOT NULL CONSTRAINT chk_alias_normalized_alias_len CHECK (length(normalized_alias) <= 1000),
    alias_type TEXT NOT NULL DEFAULT '' CONSTRAINT chk_alias_alias_type_len CHECK (length(alias_type) <= 100),
    confidence REAL NOT NULL DEFAULT 1.0 CONSTRAINT chk_alias_confidence_range CHECK (confidence >= 0 AND confidence <= 1),
    source TEXT NOT NULL DEFAULT '' CONSTRAINT chk_alias_source_len CHECK (length(source) <= 255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, node_id, normalized_alias)
);

ALTER TABLE kg_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_aliases ON kg_aliases
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE INDEX idx_aliases_tenant_node ON kg_aliases (tenant_id, node_id, created_at DESC);
CREATE INDEX idx_aliases_tenant_normalized ON kg_aliases (tenant_id, normalized_alias);
CREATE INDEX idx_aliases_tenant_type ON kg_aliases (tenant_id, alias_type);

-- +goose Down
DROP TABLE IF EXISTS kg_aliases;
