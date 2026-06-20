-- +goose Up
-- Logical namespaces: a per-tenant organizing axis (e.g. scout:, claude:, work:,
-- personal:). This was an informal id-prefix convention; make it a real column so
-- memory_search / memory_get / memory_write can filter and scope cleanly without
-- string-parsing the id. Existing rows fall into the 'default' namespace.
ALTER TABLE notes
    ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default'
    CONSTRAINT chk_note_namespace_len CHECK (length(namespace) <= 128);

-- Filtering is always tenant-scoped, so the composite index matches the query
-- shape (RLS pins tenant_id, the namespace narrows within it).
CREATE INDEX idx_notes_tenant_namespace ON notes (tenant_id, namespace);

-- +goose Down
DROP INDEX IF EXISTS idx_notes_tenant_namespace;
ALTER TABLE notes DROP COLUMN IF EXISTS namespace;
