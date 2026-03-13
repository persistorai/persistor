-- +goose Up
CREATE TABLE unknown_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    relation_type TEXT NOT NULL,
    source_name TEXT NOT NULL,
    target_name TEXT NOT NULL,
    source_text TEXT,
    count INTEGER DEFAULT 1,
    first_seen TIMESTAMPTZ DEFAULT now(),
    last_seen TIMESTAMPTZ DEFAULT now(),
    resolved BOOLEAN DEFAULT FALSE,
    resolved_as TEXT,
    UNIQUE(tenant_id, relation_type, source_name, target_name)
);
-- Enable RLS
ALTER TABLE unknown_relations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON unknown_relations
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- +goose Down
DROP TABLE IF EXISTS unknown_relations;
