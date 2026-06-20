-- +goose Up
-- Harden the note_versions append-only guarantee against TRUNCATE.
--
-- Migration 003 blocks UPDATE/DELETE with a FOR EACH ROW trigger, but TRUNCATE
-- is a statement-level operation that a row trigger never sees — so the audit
-- log could still be wiped wholesale. Add a statement-level BEFORE TRUNCATE
-- guard (reusing the same raise-exception function) to close that gap.
--
-- Note: a role that OWNS the table can still DISABLE TRIGGER or DROP the
-- function. Fully enforcing immutability additionally requires serving from a
-- non-owner role granted only INSERT/SELECT on note_versions (a deploy/ops
-- change tracked for the hosted phase); this migration closes the in-SQL gap.

CREATE TRIGGER note_versions_no_truncate
    BEFORE TRUNCATE ON note_versions
    FOR EACH STATEMENT EXECUTE FUNCTION note_versions_append_only();

-- +goose Down
DROP TRIGGER IF EXISTS note_versions_no_truncate ON note_versions;
