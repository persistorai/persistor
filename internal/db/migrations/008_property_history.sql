-- +goose Up
-- Property history tracking — originally migration 002.
-- All objects already exist from the original run; this is a no-op renumber
-- to resolve duplicate version numbers in the migration sequence.
-- The table, indexes, and RLS policy were created by the original 002_property_history.sql.

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation_property_history ON kg_property_history;
DROP TABLE IF EXISTS kg_property_history;
