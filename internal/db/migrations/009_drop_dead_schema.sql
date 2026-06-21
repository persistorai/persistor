-- +goose Up
-- Drop retired-subsystem residue. All four objects are confirmed dead by both
-- code (no Go reads/writes them on any live path) and data (0 rows / always-empty):
--
--   * sources           -- file-indexer tracking table. The filesystem note store
--                          and the reindex cron were retired in the v0.4.0
--                          Postgres-native cutover. No Go code references it.
--   * api_keys          -- StaticTokenAuth (P2), replaced by OIDC (P3). Never
--                          created or read for auth; only a tenant-purge DELETE.
--   * links             -- never-populated "[[wikilink]]" placeholder; no reader.
--   * notes.source_path -- always '' since the file store went away. Its only use
--                          was vestigial `source_path = ''` filters that separated
--                          PG-native notes from file-imported ones (none remain).
--                          idx_notes_tenant_source dies with the column.
--
-- notes / note_versions / chunks (the working set) and tenants / identities
-- (auth principals) are untouched. RLS stays.

DROP INDEX IF EXISTS idx_notes_tenant_source;
ALTER TABLE notes DROP COLUMN IF EXISTS source_path;

DROP TABLE IF EXISTS links;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS sources;

-- +goose Down
-- Best-effort restore of the dropped structure (the tables were empty and the
-- column was uniformly ''; no data is recoverable because none existed).

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

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    key_hash     TEXT NOT NULL CONSTRAINT chk_apikey_hash_len CHECK (length(key_hash) = 64),
    label        TEXT NOT NULL DEFAULT '' CONSTRAINT chk_apikey_label_len CHECK (length(label) <= 200),
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_api_keys_hash ON api_keys (key_hash);
CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id);

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

ALTER TABLE notes ADD COLUMN source_path TEXT NOT NULL DEFAULT ''
    CONSTRAINT chk_note_src_len CHECK (length(source_path) <= 4096);
CREATE INDEX idx_notes_tenant_source ON notes (tenant_id, source_path);
