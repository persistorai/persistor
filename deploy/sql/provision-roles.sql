-- Persistor production provisioning — run ONCE as a superuser (self-host) or
-- rds_superuser (managed) on a freshly created, empty database, in two phases
-- around `persistor migrate`. Replace the CHANGEME passwords first.
--
-- Role model (resolves the SQL review's role-separation finding): migrations run
-- as a schema-OWNING migrator role; the daemon connects as a separate, non-owner,
-- least-privilege app role. FORCE ROW LEVEL SECURITY already subjects even the
-- owner to RLS, and dbpool refuses a SUPERUSER/BYPASSRLS role — but only a
-- non-owner is barred from ALTER TABLE ... DISABLE TRIGGER and DROP POLICY, which
-- is what protects the append-only note_versions audit log. persistor-server
-- asserts at boot (with PERSISTOR_AUTO_MIGRATE=false) that it is not the owner.
--
-- The single-role self-host posture (PERSISTOR_AUTO_MIGRATE=true, the default)
-- does not need this split: one owning role both migrates and serves. Use this
-- script for the hardened production posture.

-- ============================================================================
-- PHASE 1 — before `persistor migrate`. Run as superuser / rds_superuser.
-- ============================================================================

-- Extensions are database-global and privileged (a managed provider restricts
-- CREATE EXTENSION; a non-owner self-host role lacks it). btree_gin backs the
-- tenant-scoped composite FTS index in migration 010, and pg_trgm backs the
-- zero-hit fuzzy-fallback index in migration 012 — install them here, not in
-- a migration.
CREATE EXTENSION IF NOT EXISTS btree_gin;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- The migrator owns the schema and runs all DDL (migrations).
CREATE ROLE persistor_migrator LOGIN PASSWORD 'CHANGEME_MIGRATOR' NOSUPERUSER NOBYPASSRLS;
ALTER SCHEMA public OWNER TO persistor_migrator;

-- The app role the daemon connects as: non-owner, least privilege.
CREATE ROLE persistor_app LOGIN PASSWORD 'CHANGEME_APP' NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO persistor_app;

-- Now run, as the migrator:
--     DATABASE_URL=postgres://persistor_migrator:...@host/persistor persistor migrate

-- ============================================================================
-- PHASE 2 — after `persistor migrate` (the tables now exist). Run as the
-- migrator/owner (or superuser).
-- ============================================================================

-- Least-privilege table grants. note_versions is append-only for the app
-- (SELECT + INSERT only); its trigger blocks UPDATE/DELETE/TRUNCATE and the
-- operator purge path runs as the owner. goose_db_version is read so the daemon
-- can verify the schema is current at boot.
GRANT SELECT, INSERT, UPDATE, DELETE ON notes, chunks, tenants, identities, note_access TO persistor_app;
GRANT SELECT, INSERT ON note_versions TO persistor_app;
GRANT SELECT ON goose_db_version TO persistor_app;

-- chunks.id defaults from gen_random_uuid() (no sequence). If a future migration
-- adds a SERIAL/IDENTITY column, also grant its sequence:
--     GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO persistor_app;

-- Finally, point the daemon's DATABASE_URL at persistor_app and set
-- PERSISTOR_AUTO_MIGRATE=false so it serves (non-owner) without trying to migrate.
