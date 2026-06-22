-- +goose NO TRANSACTION
--
-- Production-hardening indexes and constraints. Marked NO TRANSACTION so the
-- index rebuilds can use CREATE INDEX CONCURRENTLY: this migration runs against
-- a populated notes/chunks table, where a non-concurrent build would take a
-- write-blocking lock for its duration. Every statement is autocommitted and
-- guarded (IF EXISTS / IF NOT EXISTS / NOT VALID + VALIDATE) so a partial
-- failure can be re-run safely. New indexes are built under fresh names and the
-- old ones dropped afterward, so there is never a window without a usable index.

-- +goose Up

-- 1. Tenant-scope the FTS index. The original gin(search_tsv) probes every
--    tenant's posting lists for a matching term before RLS prunes, so search
--    latency scaled with the global corpus rather than the tenant's. The
--    composite GIN lets one scan satisfy `tenant_id = $1 AND search_tsv @@ $2`.
--    It needs the btree_gin operator class for the uuid column. CREATE EXTENSION
--    is a privileged, database-global operation (a managed provider like RDS
--    restricts it; a self-host non-owner role lacks it too), so btree_gin is a
--    PROVISIONING step run as superuser BEFORE migrating — not part of this
--    migration. See the README Quick start and deploy/sql/provision-roles.sql.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_tenant_tsv
    ON chunks USING gin (tenant_id, search_tsv);

DROP INDEX CONCURRENTLY IF EXISTS idx_chunks_search_tsv;

-- 2. Align the partial-index predicates with the live read predicate. Active
--    reads now filter `deleted = FALSE AND superseded = FALSE`, but the 002
--    indexes predicated on superseded alone, so tombstoned rows stayed in the
--    hot indexes and were filtered only after the index scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_core_v2
    ON notes (tenant_id, id)
    WHERE tier = 'core' AND superseded = FALSE AND deleted = FALSE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_active_v2
    ON notes (tenant_id)
    WHERE superseded = FALSE AND deleted = FALSE;

DROP INDEX CONCURRENTLY IF EXISTS idx_notes_core;
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_active;

-- 3. Index supersedes so supersession reconcile (and any supersedes lookup) no
--    longer sequential-scans the tenant's notes on every write/delete/restore.
--    Partial: only the superseding rows carry a non-NULL supersedes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_tenant_supersedes
    ON notes (tenant_id, supersedes)
    WHERE supersedes IS NOT NULL;

-- 4. Defend the chunk projection's shape. replaceChunks always deletes a note's
--    chunks before reinserting, so (tenant_id, note_id, ord) is unique today;
--    the constraint makes a future insert-without-delete fail loudly instead of
--    silently duplicating, and documents the intended shape.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_chunks_tenant_note_ord
    ON chunks (tenant_id, note_id, ord);

-- 5. Bound note body size at the DB boundary, paired with the daemon's request-
--    body cap. 1 MiB is far above any real prose note but stops a single
--    authenticated writer from exhausting disk with one giant memory_write.
--    NOT VALID + VALIDATE avoids a long ACCESS EXCLUSIVE lock on the populated
--    table: the validation scan takes only SHARE UPDATE EXCLUSIVE.
ALTER TABLE notes ADD CONSTRAINT chk_note_body_len CHECK (length(body) <= 1048576) NOT VALID;
ALTER TABLE notes VALIDATE CONSTRAINT chk_note_body_len;

-- 6. Tune autovacuum for the churn tables. replaceChunks dead-tuples a note's
--    whole chunk set on every write; the default 0.2 scale factor lets bloat
--    accumulate and degrade the GIN scan before autovacuum triggers.
ALTER TABLE chunks SET (autovacuum_vacuum_scale_factor = 0.05, autovacuum_analyze_scale_factor = 0.02);
ALTER TABLE notes  SET (autovacuum_vacuum_scale_factor = 0.05, autovacuum_analyze_scale_factor = 0.02);

-- +goose Down
ALTER TABLE notes  RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE chunks RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);

ALTER TABLE notes DROP CONSTRAINT IF EXISTS chk_note_body_len;

DROP INDEX CONCURRENTLY IF EXISTS uq_chunks_tenant_note_ord;
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_tenant_supersedes;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_active
    ON notes (tenant_id) WHERE superseded = FALSE;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_core
    ON notes (tenant_id, id) WHERE tier = 'core' AND superseded = FALSE;
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_active_v2;
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_core_v2;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_search_tsv
    ON chunks USING gin (search_tsv);
DROP INDEX CONCURRENTLY IF EXISTS idx_chunks_tenant_tsv;
-- btree_gin is left installed on the down path (harmless, may be shared).
