-- +goose NO TRANSACTION
-- +goose Up
-- Persistor - Initial Schema
-- Requires PostgreSQL 18+ with pgvector extension.

CREATE EXTENSION IF NOT EXISTS vector;

-- Tenants.
CREATE TABLE tenants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL CONSTRAINT chk_tenant_name_len CHECK (length(name) <= 255),
    api_key_hash  TEXT NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    plan          TEXT NOT NULL DEFAULT 'free' CONSTRAINT chk_tenant_plan_len CHECK (length(plan) <= 50),
    encrypted     BOOLEAN NOT NULL DEFAULT TRUE
);

-- Knowledge graph nodes.
CREATE TABLE kg_nodes (
    id              TEXT NOT NULL CONSTRAINT chk_node_id_len CHECK (length(id) <= 255),
    tenant_id       UUID NOT NULL,
    type            TEXT NOT NULL CONSTRAINT chk_node_type_len CHECK (length(type) <= 100),
    label           TEXT NOT NULL CONSTRAINT chk_node_label_len CHECK (length(label) <= 10000),
    properties      JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1024),

    -- Salience tracking.
    access_count    INTEGER NOT NULL DEFAULT 0,
    last_accessed   TIMESTAMPTZ,
    salience_score  REAL NOT NULL DEFAULT 1.0 CONSTRAINT chk_node_salience_nonneg CHECK (salience_score >= 0),
    superseded_by   TEXT,
    user_boosted    BOOLEAN NOT NULL DEFAULT FALSE,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Pre-computed tsvector for full-text search (avoids per-query recomputation).
    label_tsv       tsvector GENERATED ALWAYS AS (to_tsvector('english', label)) STORED,

    PRIMARY KEY (tenant_id, id)
);

-- RLS for nodes.
ALTER TABLE kg_nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_nodes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_nodes ON kg_nodes
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Knowledge graph edges (no foreign keys — referential integrity in app layer).
CREATE TABLE kg_edges (
    tenant_id       UUID NOT NULL,
    source          TEXT NOT NULL CONSTRAINT chk_edge_source_len CHECK (length(source) <= 255),
    target          TEXT NOT NULL CONSTRAINT chk_edge_target_len CHECK (length(target) <= 255),
    relation        TEXT NOT NULL CONSTRAINT chk_edge_relation_len CHECK (length(relation) <= 255),
    properties      JSONB NOT NULL DEFAULT '{}',
    weight          REAL NOT NULL DEFAULT 1.0,

    -- Salience tracking.
    access_count    INTEGER NOT NULL DEFAULT 0,
    last_accessed   TIMESTAMPTZ,
    salience_score  REAL NOT NULL DEFAULT 1.0 CONSTRAINT chk_edge_salience_nonneg CHECK (salience_score >= 0),
    superseded_by   TEXT,
    user_boosted    BOOLEAN NOT NULL DEFAULT FALSE,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (tenant_id, source, target, relation)
);

-- RLS for edges.
ALTER TABLE kg_edges ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_edges FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_edges ON kg_edges
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Node indexes.
-- All indexes use CONCURRENTLY to avoid locking tables; migration runs with NO TRANSACTION.
CREATE INDEX CONCURRENTLY idx_nodes_tenant_type ON kg_nodes(tenant_id, type);
CREATE INDEX CONCURRENTLY idx_nodes_tenant_salience_updated ON kg_nodes(tenant_id, salience_score DESC, updated_at DESC);
CREATE INDEX CONCURRENTLY idx_nodes_tenant_type_salience ON kg_nodes(tenant_id, type, salience_score DESC, updated_at DESC);
CREATE INDEX CONCURRENTLY idx_nodes_tenant_updated ON kg_nodes(tenant_id, updated_at);
-- HNSW parameters tuned for Qwen3-Embedding-0.6B 1024-dimensional vectors.
-- m = 32 and ef_construction = 200 improve recall at the cost of slower index builds.
CREATE INDEX CONCURRENTLY idx_nodes_embedding ON kg_nodes USING hnsw (embedding vector_cosine_ops)
    WITH (m = 32, ef_construction = 200) WHERE embedding IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_nodes_fts ON kg_nodes USING gin (label_tsv);

-- Edge indexes.
CREATE INDEX CONCURRENTLY idx_edges_tenant_source ON kg_edges(tenant_id, source);
CREATE INDEX CONCURRENTLY idx_edges_tenant_target ON kg_edges(tenant_id, target);
CREATE INDEX CONCURRENTLY idx_edges_tenant_relation ON kg_edges(tenant_id, relation);
CREATE INDEX CONCURRENTLY idx_edges_tenant_source_relation ON kg_edges(tenant_id, source, relation);
CREATE INDEX CONCURRENTLY idx_edges_tenant_updated ON kg_edges(tenant_id, updated_at);

-- Auto-update updated_at.
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER nodes_updated BEFORE UPDATE ON kg_nodes
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

CREATE TRIGGER edges_updated BEFORE UPDATE ON kg_edges
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

-- KG change notifications are sent from the application layer (see db/nodes.go,
-- db/edges.go, db/bulk.go) via pg_notify after transaction commit.

-- Default tenant for local/self-hosted use.
INSERT INTO tenants (name, api_key_hash, plan)
VALUES ('default', encode(sha256(('local-' || gen_random_uuid()::text)::bytea), 'hex'), 'self-hosted');

-- +goose Down
DROP TRIGGER IF EXISTS edges_updated ON kg_edges;
DROP TRIGGER IF EXISTS nodes_updated ON kg_nodes;
DROP FUNCTION IF EXISTS update_timestamp();
DROP TABLE IF EXISTS kg_edges;
DROP TABLE IF EXISTS kg_nodes;
DROP TABLE IF EXISTS tenants;
DROP EXTENSION IF EXISTS vector;
