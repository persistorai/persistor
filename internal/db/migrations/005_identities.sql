-- +goose Up
-- Tenancy & onboarding (Phase P4). Two RLS-EXEMPT admin tables that route an IdP
-- identity to a tenant. Like api_keys (migration 004), they are consulted in the
-- auth path BEFORE app.tenant_id exists — resolving the identity is precisely how
-- the tenant is DISCOVERED, a chicken-and-egg a tenant-scoped policy cannot
-- satisfy. They hold NO note content: only identity->tenant routing and a human
-- label. Reviewed as a deliberate RLS exemption, same rationale as api_keys.

CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label      TEXT NOT NULL DEFAULT '' CONSTRAINT chk_tenant_label_len CHECK (length(label) <= 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One identity == one (issuer, subject) pair from the IdP. On first login the
-- OIDC auth path auto-provisions a row whose tenant is the claim-derived
-- uuidv5(iss|sub) — preserving the pre-P4 stateless mapping so existing users
-- keep their memory. An admin can instead pre-map an identity to a chosen tenant
-- (the work-vs-personal separation case) before first login.
CREATE TABLE identities (
    issuer       TEXT NOT NULL CONSTRAINT chk_identity_issuer_len CHECK (length(issuer) BETWEEN 1 AND 500),
    subject      TEXT NOT NULL CONSTRAINT chk_identity_subject_len CHECK (length(subject) BETWEEN 1 AND 500),
    tenant_id    UUID NOT NULL,
    role         TEXT NOT NULL DEFAULT 'owner'
                 CONSTRAINT chk_identity_role CHECK (role IN ('owner', 'member', 'readonly')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ,
    PRIMARY KEY (issuer, subject)
);

-- List/offboard every identity routed to a tenant.
CREATE INDEX idx_identities_tenant ON identities (tenant_id);

-- Note: intentionally NO "ENABLE ROW LEVEL SECURITY" on either table. See the
-- header comment — this is a reviewed RLS exemption, like api_keys.

-- +goose Down
DROP TABLE IF EXISTS identities;
DROP TABLE IF EXISTS tenants;
