-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE kg_nodes
    ADD COLUMN search_text TEXT NOT NULL DEFAULT '';

UPDATE kg_nodes
SET search_text = trim(
    concat_ws(E'\n',
        label,
        type,
        COALESCE(
            (
                SELECT string_agg(value_text, E'\n')
                FROM (
                    SELECT trim(value #>> '{}') AS value_text
                    FROM jsonb_each(properties)
                    WHERE left(key, 1) <> '_'
                    ORDER BY key
                    LIMIT 24
                ) property_values
                WHERE value_text <> ''
            ),
            ''
        )
    )
);

ALTER TABLE kg_nodes
    DROP COLUMN label_tsv,
    ADD COLUMN search_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', search_text)) STORED;

DROP INDEX IF EXISTS idx_nodes_fts;
CREATE INDEX CONCURRENTLY idx_nodes_fts ON kg_nodes USING gin (search_tsv);

-- +goose Down
DROP INDEX IF EXISTS idx_nodes_fts;
CREATE INDEX CONCURRENTLY idx_nodes_fts ON kg_nodes USING gin (label_tsv);

ALTER TABLE kg_nodes
    DROP COLUMN search_tsv,
    ADD COLUMN label_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', label)) STORED;

ALTER TABLE kg_nodes
    DROP COLUMN IF EXISTS search_text;
