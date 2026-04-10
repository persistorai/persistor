-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE kg_episodes (
    id                        TEXT NOT NULL CONSTRAINT chk_episode_id_len CHECK (length(id) <= 255),
    tenant_id                 UUID NOT NULL,
    title                     TEXT NOT NULL CONSTRAINT chk_episode_title_len CHECK (length(title) <= 10000),
    summary                   TEXT NOT NULL DEFAULT '',
    status                    TEXT NOT NULL DEFAULT 'open' CONSTRAINT chk_episode_status CHECK (status IN ('open', 'closed')),
    started_at                TIMESTAMPTZ,
    ended_at                  TIMESTAMPTZ,
    primary_project_node_id   TEXT,
    source_artifact_node_id   TEXT,
    properties                JSONB NOT NULL DEFAULT '{}',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (tenant_id, id),
    CONSTRAINT chk_episode_time_range CHECK (started_at IS NULL OR ended_at IS NULL OR ended_at >= started_at)
);

ALTER TABLE kg_episodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_episodes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_episodes ON kg_episodes
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TABLE kg_event_records (
    id                        TEXT NOT NULL CONSTRAINT chk_event_id_len CHECK (length(id) <= 255),
    tenant_id                 UUID NOT NULL,
    episode_id                TEXT,
    parent_event_id           TEXT,
    kind                      TEXT NOT NULL CONSTRAINT chk_event_kind CHECK (kind IN ('observation', 'conversation', 'message', 'decision', 'task', 'promise', 'outcome')),
    title                     TEXT NOT NULL CONSTRAINT chk_event_title_len CHECK (length(title) <= 10000),
    summary                   TEXT NOT NULL DEFAULT '',
    occurred_at               TIMESTAMPTZ,
    occurred_start_at         TIMESTAMPTZ,
    occurred_end_at           TIMESTAMPTZ,
    confidence                REAL NOT NULL DEFAULT 1.0 CONSTRAINT chk_event_confidence CHECK (confidence >= 0 AND confidence <= 1),
    evidence                  JSONB NOT NULL DEFAULT '[]',
    source_artifact_node_id   TEXT,
    properties                JSONB NOT NULL DEFAULT '{}',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (tenant_id, id),
    CONSTRAINT chk_event_time_range CHECK (occurred_start_at IS NULL OR occurred_end_at IS NULL OR occurred_end_at >= occurred_start_at)
);

ALTER TABLE kg_event_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_event_records FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_event_records ON kg_event_records
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TABLE kg_event_links (
    tenant_id    UUID NOT NULL,
    event_id     TEXT NOT NULL,
    node_id      TEXT NOT NULL,
    role         TEXT NOT NULL CONSTRAINT chk_event_link_role_len CHECK (length(role) <= 255),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (tenant_id, event_id, node_id, role)
);

ALTER TABLE kg_event_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_event_links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_event_links ON kg_event_links
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE INDEX CONCURRENTLY idx_episodes_tenant_status_updated ON kg_episodes(tenant_id, status, updated_at DESC);
CREATE INDEX CONCURRENTLY idx_episodes_tenant_started ON kg_episodes(tenant_id, started_at DESC NULLS LAST, created_at DESC);
CREATE INDEX CONCURRENTLY idx_event_records_tenant_kind_occurred ON kg_event_records(tenant_id, kind, occurred_at DESC NULLS LAST, created_at DESC);
CREATE INDEX CONCURRENTLY idx_event_records_tenant_episode ON kg_event_records(tenant_id, episode_id, occurred_at DESC NULLS LAST, created_at DESC);
CREATE INDEX CONCURRENTLY idx_event_records_tenant_parent ON kg_event_records(tenant_id, parent_event_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_event_links_tenant_node ON kg_event_links(tenant_id, node_id, created_at DESC);

CREATE TRIGGER episodes_updated BEFORE UPDATE ON kg_episodes
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

CREATE TRIGGER event_records_updated BEFORE UPDATE ON kg_event_records
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

-- +goose Down
DROP TRIGGER IF EXISTS event_records_updated ON kg_event_records;
DROP TRIGGER IF EXISTS episodes_updated ON kg_episodes;
DROP TABLE IF EXISTS kg_event_links;
DROP TABLE IF EXISTS kg_event_records;
DROP TABLE IF EXISTS kg_episodes;
