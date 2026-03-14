-- +goose Up
ALTER TABLE kg_edges
    ADD COLUMN date_start      TEXT    DEFAULT NULL,
    ADD COLUMN date_end        TEXT    DEFAULT NULL,
    ADD COLUMN date_lower      DATE    DEFAULT NULL,
    ADD COLUMN date_upper      DATE    DEFAULT NULL,
    ADD COLUMN is_current      BOOLEAN DEFAULT NULL,
    ADD COLUMN date_qualifier  TEXT    DEFAULT NULL;

CREATE INDEX idx_kg_edges_date_lower ON kg_edges (tenant_id, date_lower) WHERE date_lower IS NOT NULL;
CREATE INDEX idx_kg_edges_date_upper ON kg_edges (tenant_id, date_upper) WHERE date_upper IS NOT NULL;
CREATE INDEX idx_kg_edges_is_current ON kg_edges (tenant_id, is_current) WHERE is_current IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_kg_edges_is_current;
DROP INDEX IF EXISTS idx_kg_edges_date_upper;
DROP INDEX IF EXISTS idx_kg_edges_date_lower;

ALTER TABLE kg_edges
    DROP COLUMN IF EXISTS date_qualifier,
    DROP COLUMN IF EXISTS is_current,
    DROP COLUMN IF EXISTS date_upper,
    DROP COLUMN IF EXISTS date_lower,
    DROP COLUMN IF EXISTS date_end,
    DROP COLUMN IF EXISTS date_start;
