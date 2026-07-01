-- +goose Up
-- Fuzzy-fallback index: when FTS finds nothing (typos, misspelled entity
-- names — "balast systm", "Meridain Acord"), search retries with trigram
-- word-similarity ($1 <%% text). This GIN index makes that retry an index scan.
-- Requires the pg_trgm extension, installed at provisioning (like btree_gin,
-- deliberately NOT in a migration: CREATE EXTENSION is privileged).
CREATE INDEX idx_chunks_text_trgm ON chunks USING gin (text gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_chunks_text_trgm;
