-- +goose Up
-- Memory index.
--
-- The prose.md notes are the source of truth (in git); these tables are a
-- rebuildable FTS index over them. Index everything; decide relevance at query
-- time. No entity graph, no vectors/embeddings, no separate tenants table (tenancy is the app.tenant_id session setting
-- enforced by RLS; there are no foreign keys to a tenant row).
--
-- Tables are created empty, so indexes use plain CREATE INDEX inside goose's
-- default transaction.
--
-- Encryption note: RLS tenant isolation is enforced on every table.
-- At-rest AES-256-GCM of note bodies is intentionally NOT applied in this
-- FTS-first build (Postgres full-text search needs a plaintext tsvector, and the
-- source notes are plaintext in a private repo on the same host). At-rest
-- encryption is a hosted/public-MCP requirement, tracked for that phase.

-- update_timestamp keeps updated_at current on row updates.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- sources: one row per indexed file. The file-sync indexer hashes each
-- watched file and re-indexes only when sha256 changes.
CREATE TABLE sources (
    tenant_id     UUID NOT NULL,
    path          TEXT NOT NULL CONSTRAINT chk_source_path_len CHECK (length(path) <= 4096),
    root          TEXT NOT NULL CONSTRAINT chk_source_root_len CHECK (length(root) <= 64),
    sha256        TEXT NOT NULL CONSTRAINT chk_source_sha_len CHECK (length(sha256) = 64),
    last_indexed  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, path)
);

ALTER TABLE sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE sources FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_sources ON sources
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- notes: a logical unit of memory backed by prose. One note maps to one source
-- file (one note per source file); source_path points at the backing file.
CREATE TABLE notes (
    id            TEXT NOT NULL CONSTRAINT chk_note_id_len CHECK (length(id) <= 512),
    tenant_id     UUID NOT NULL,
    kind          TEXT NOT NULL DEFAULT 'fact'
                  CONSTRAINT chk_note_kind CHECK (kind IN ('fact','decision','episode','reference','preference')),
    tier          TEXT NOT NULL DEFAULT 'tail'
                  CONSTRAINT chk_note_tier CHECK (tier IN ('core','tail')),
    title         TEXT NOT NULL DEFAULT '' CONSTRAINT chk_note_title_len CHECK (length(title) <= 1000),
    body          TEXT NOT NULL DEFAULT '',
    source_path   TEXT NOT NULL DEFAULT '' CONSTRAINT chk_note_src_len CHECK (length(source_path) <= 4096),
    supersedes    TEXT,
    superseded    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, id)
);

ALTER TABLE notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE notes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_notes ON notes
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- chunks: the searchable unit. text is plaintext (FTS needs it); search_tsv is a
-- generated tsvector indexed with GIN. No embedding column — vectors are out.
CREATE TABLE chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id       TEXT NOT NULL CONSTRAINT chk_chunk_note_len CHECK (length(note_id) <= 512),
    tenant_id     UUID NOT NULL,
    ord           INTEGER NOT NULL DEFAULT 0,
    text          TEXT NOT NULL DEFAULT '',
    search_tsv    tsvector GENERATED ALWAYS AS (to_tsvector('english', text)) STORED,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_chunks ON chunks
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- links: optional write-time associations between notes. Empty until
-- shows they help; carried so the option exists.
CREATE TABLE links (
    tenant_id       UUID NOT NULL,
    note_id         TEXT NOT NULL CONSTRAINT chk_link_src_len CHECK (length(note_id) <= 512),
    target_note_id  TEXT NOT NULL CONSTRAINT chk_link_tgt_len CHECK (length(target_note_id) <= 512),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, note_id, target_note_id)
);

ALTER TABLE links ENABLE ROW LEVEL SECURITY;
ALTER TABLE links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_links ON links
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Indexes.
CREATE INDEX idx_notes_tenant_source ON notes (tenant_id, source_path);
CREATE INDEX idx_notes_tenant_superseded ON notes (tenant_id, superseded);
CREATE INDEX idx_chunks_tenant_note ON chunks (tenant_id, note_id);
CREATE INDEX idx_chunks_search_tsv ON chunks USING gin (search_tsv);

CREATE TRIGGER notes_updated BEFORE UPDATE ON notes
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

-- +goose Down
DROP TRIGGER IF EXISTS notes_updated ON notes;
DROP TABLE IF EXISTS links;
DROP TABLE IF EXISTS chunks;
DROP TABLE IF EXISTS notes;
DROP TABLE IF EXISTS sources;
DROP FUNCTION IF EXISTS update_timestamp();
