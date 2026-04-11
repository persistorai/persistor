-- +goose Up
ALTER TABLE tenants
    ADD COLUMN api_key_scope TEXT NOT NULL DEFAULT 'read_write',
    ADD CONSTRAINT chk_tenants_api_key_scope CHECK (api_key_scope IN ('read_write', 'admin'));

-- +goose Down
ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS chk_tenants_api_key_scope,
    DROP COLUMN IF EXISTS api_key_scope;
