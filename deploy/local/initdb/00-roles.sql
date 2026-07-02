-- Local dev role provisioning — runs ONCE via the postgres image's
-- docker-entrypoint-initdb.d, as the superuser, against the `persistor` database.
--
-- This is the LOCAL mirror of deploy/sql/provision-roles.sql. It reproduces
-- production's split-role posture so a locally-run server exercises the same
-- boot path and RLS enforcement that ships:
--   * persistor_migrator  — owns the schema, runs migrations (the migrate service)
--   * persistor_app       — non-owner, NOSUPERUSER NOBYPASSRLS (the daemon)
--
-- Passwords are trivially guessable ON PURPOSE: this DB is bound to 127.0.0.1,
-- holds only throwaway test data, and is wiped by `make dev-reset`. Never reuse
-- this file's posture anywhere reachable.

-- Extensions are database-global and privileged; the migrations assume they
-- already exist (btree_gin backs the composite FTS index in migration 010,
-- pg_trgm backs the fuzzy-fallback index in migration 012). Install them here,
-- before any migration runs — same as the prod provisioning script.
CREATE EXTENSION IF NOT EXISTS btree_gin;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Schema owner / migrator.
CREATE ROLE persistor_migrator LOGIN PASSWORD 'persistor_migrator' NOSUPERUSER NOBYPASSRLS;
ALTER SCHEMA public OWNER TO persistor_migrator;

-- The daemon's role: non-owner, least privilege. Being a non-owner is what bars
-- it from ALTER TABLE ... DISABLE TRIGGER / DROP POLICY (protecting the
-- append-only note_versions audit log); FORCE ROW LEVEL SECURITY subjects it to
-- RLS regardless, and NOBYPASSRLS is asserted by the daemon's dbpool at boot.
CREATE ROLE persistor_app LOGIN PASSWORD 'persistor_app' NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO persistor_app;

-- Grant the app role its table rights up front via DEFAULT PRIVILEGES, so the
-- tables the migrator is about to create are automatically granted — no
-- post-migrate grant step needed (the one simplification vs. the prod script,
-- which tightens note_versions to INSERT-only and goose_db_version to SELECT).
-- Here the app gets full DML on every migrator-created table; the append-only
-- guarantee on note_versions still holds because it is enforced by a trigger,
-- and RLS/NOBYPASSRLS still enforce tenant isolation. Good enough for a
-- throwaway local box; do NOT copy this looseness to production.
ALTER DEFAULT PRIVILEGES FOR ROLE persistor_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO persistor_app;
ALTER DEFAULT PRIVILEGES FOR ROLE persistor_migrator IN SCHEMA public
  GRANT SELECT, USAGE ON SEQUENCES TO persistor_app;
