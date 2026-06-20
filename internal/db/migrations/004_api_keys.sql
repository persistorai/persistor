-- +goose Up
-- API keys for the remote MCP daemon (Phase P2: StaticTokenAuth).
--
-- This table maps a bearer token to the tenant it authenticates. It is the auth
-- oracle the HTTP middleware consults on every request, BEFORE any tenant
-- context exists — the whole point of the lookup is to discover which tenant a
-- token belongs to. That ordering makes it the one deliberate exception to the
-- "every table gets FORCE RLS" rule:
--
--   * It is RLS-EXEMPT on purpose. A tenant-scoped RLS policy would require
--     app.tenant_id to already be set, but resolving the token is precisely how
--     we learn the tenant — a chicken-and-egg the policy cannot satisfy. So the
--     auth path queries this table with no tenant GUC, as the table owner.
--   * It holds NO tenant content — only a token->tenant mapping. The secret is
--     the raw token, which is NEVER stored: key_hash is sha256(token) hex (64
--     chars). Tokens are high-entropy random, so an unsalted SHA-256 is
--     appropriate here (unlike passwords).
--   * A leaked DB read therefore exposes hashes and tenant ids, not usable
--     tokens, and no note bodies.
--
-- Revocation is a flag (revoked) rather than a delete, so a compromised key can
-- be disabled while keeping an audit trail (created_at / last_used_at).

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    key_hash     TEXT NOT NULL CONSTRAINT chk_apikey_hash_len CHECK (length(key_hash) = 64),
    label        TEXT NOT NULL DEFAULT '' CONSTRAINT chk_apikey_label_len CHECK (length(label) <= 200),
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- One row per token; the auth lookup is an equality probe on the hash.
CREATE UNIQUE INDEX idx_api_keys_hash ON api_keys (key_hash);
-- List/revoke a tenant's keys.
CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id);

-- Note: intentionally NO "ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY". See
-- the header comment. This table is reviewed as a deliberate RLS exemption.

-- +goose Down
DROP TABLE IF EXISTS api_keys;
