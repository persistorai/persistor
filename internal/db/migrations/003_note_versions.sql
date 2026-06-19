-- +goose Up
-- Notes as source of truth + append-only version history.
--
-- This is the storage foundation for PG-native tenants (the hosted, remote-MCP
-- path). For those tenants the notes row IS the source of truth (source_path is
-- empty, there is no backing file), so we need:
--   1. concurrency control, so two surfaces (laptop, phone, web) writing the same
--      note don't silently clobber each other -> notes.version + optimistic checks,
--   2. recoverability, so an accidental delete or a bad overwrite can be undone ->
--      an append-only note_versions log that also doubles as the write/supersede/
--      delete audit trail the hosted-phase checklist requires,
--   3. tombstone deletes, so the query path never needs a soft-delete flag beyond a
--      single boolean filtered out of live reads.
--
-- File-backed namespaces (scout:, claude:) are unaffected: their notes keep
-- source_path set and version stays 1 unless a PG-native write touches them (it
-- won't). The columns are additive with constant DEFAULTs, so ADD COLUMN is a
-- metadata-only change on PostgreSQL 11+ (no table rewrite) and runs safely inside
-- goose's transaction.

-- New columns on notes. version drives optimistic concurrency; deleted is the
-- tombstone read by live queries; updated_by records the surface/client of the
-- last write (audit).
ALTER TABLE notes
    ADD COLUMN version    INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN deleted    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN updated_by TEXT;

-- note_versions: append-only history. One row per write to a note (create,
-- update, delete, restore), capturing the resulting note state. PK
-- (tenant_id, note_id, version) makes history a tenant-scoped range scan and
-- enforces one row per version. Same RLS posture as every other table.
CREATE TABLE note_versions (
    tenant_id   UUID NOT NULL,
    note_id     TEXT NOT NULL CONSTRAINT chk_nv_note_len CHECK (length(note_id) <= 512),
    version     INTEGER NOT NULL,
    title       TEXT NOT NULL DEFAULT '' CONSTRAINT chk_nv_title_len CHECK (length(title) <= 1000),
    body        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT 'fact'
                CONSTRAINT chk_nv_kind CHECK (kind IN ('fact','decision','episode','reference','preference')),
    tier        TEXT NOT NULL DEFAULT 'tail'
                CONSTRAINT chk_nv_tier CHECK (tier IN ('core','tail')),
    op          TEXT NOT NULL
                CONSTRAINT chk_nv_op CHECK (op IN ('create','update','delete','restore')),
    surface     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, note_id, version)
);

ALTER TABLE note_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_note_versions ON note_versions
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Append-only guard: the version log is an audit record. Block UPDATE/DELETE at
-- the database so neither a bug nor a prompt-injected write path can rewrite
-- history; the only legal mutation is INSERT. Restore is itself a new INSERT (a
-- prior version copied forward), so this never gets in the way of a real flow.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION note_versions_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'note_versions is append-only: % not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER note_versions_no_update
    BEFORE UPDATE OR DELETE ON note_versions
    FOR EACH ROW EXECUTE FUNCTION note_versions_append_only();

-- +goose Down
DROP TRIGGER IF EXISTS note_versions_no_update ON note_versions;
DROP FUNCTION IF EXISTS note_versions_append_only();
DROP TABLE IF EXISTS note_versions;
ALTER TABLE notes
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS deleted,
    DROP COLUMN IF EXISTS version;
