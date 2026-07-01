-- +goose Up
-- Access telemetry, kept OUT of notes on purpose: the update_timestamp trigger
-- bumps notes.updated_at on every UPDATE, so counting reads there would
-- corrupt freshness semantics (updated_at means content changed, and F2 ranks
-- by it). Telemetry is observational — it informs ops and future decisions
-- (e.g. whether decay is ever justified) and is deliberately NOT a ranking
-- input.
CREATE TABLE note_access (
    tenant_id        UUID NOT NULL,
    note_id          TEXT NOT NULL CONSTRAINT chk_access_note_len CHECK (length(note_id) <= 512),
    access_count     BIGINT NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, note_id)
);

ALTER TABLE note_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE note_access FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_note_access ON note_access
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Split-role posture: the migrator owns this table; grant the serving role its
-- working set. Conditional so the single-role self-host (no persistor_app
-- role) migrates cleanly. Mirrored in deploy/sql/provision-roles.sql for
-- fresh provisions.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'persistor_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON note_access TO persistor_app;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS note_access;
