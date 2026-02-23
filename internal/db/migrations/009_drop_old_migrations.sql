-- +goose Up
DROP TABLE IF EXISTS schema_migrations;

-- +goose Down
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
