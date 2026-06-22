-- +goose NO TRANSACTION
--
-- NO TRANSACTION so the index build can use CREATE INDEX CONCURRENTLY against the
-- populated notes table without taking a write-blocking lock.

-- +goose Up
-- ListNotes pages a namespace with `WHERE tenant_id = ? AND namespace = ?
-- ORDER BY id LIMIT/OFFSET`. The (tenant_id, namespace) index from 007 doesn't
-- carry id, so that ORDER BY needed an external sort. Add id so the index orders
-- the page directly. (A non-namespace list is already ordered by the PK
-- (tenant_id, id); deep OFFSET paging remains O(offset) but no longer sorts.)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notes_tenant_namespace_id
    ON notes (tenant_id, namespace, id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_notes_tenant_namespace_id;
