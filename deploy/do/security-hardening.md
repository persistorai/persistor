# Persistor production security hardening

Tracks the findings from the 2026-06-23 security review of the live DigitalOcean
deployment (`mcp.persistor.ai`) and their disposition. The confidentiality
posture (OIDC verification, fail-closed tenant derivation, `FORCE`d RLS, the
`NOSUPERUSER NOBYPASSRLS` app role, firewalled DB) passed with no critical/high
issue. The items below are availability / public-vs-tailnet hardening — the code
was originally written for a tailnet-bound daemon.

## Shipped in v0.9.2 (app-side)

- **`/metrics` is bearer-gated** (was public). It exposed DB pool capacity
  (`max_conns`/`acquired_conns`) and traffic/error counts on a public ingress;
  now it requires a valid token like `/mcp`. Health checks use `/healthz`
  (unauthenticated, no body), so this doesn't affect App Platform liveness.
- **`memory_search` / `brief` result limits are capped at 500** (mirroring
  `memory_list`), plus a defensive cap in `index.SearchNotes`. Closes a
  one-request DoS via `LIMIT 1e9`.
- **`/mcp` has a coarse per-IP backstop + a `WriteTimeout`.** The per-tenant
  limiters run post-auth; the new per-IP limiter (100 rps, burst 200) drops a
  pre-auth flood before JWT verification. Behind a proxy it collapses toward a
  global limit (see below). `WriteTimeout=120s` bounds slow-read clients (safe:
  the transport is non-streaming JSON).
- **Tenant-derivation separator guard.** `Verify` rejects a token subject
  containing the `|` join separator, foreclosing a future multi-issuer tenant
  collision. The derivation byte string is unchanged, so existing tenant ids are
  stable (no data migration).
- **Generic internal errors at the tool boundary.** Store/DB errors are logged
  server-side and replaced with a generic `internal error` for the client, so
  Postgres schema/SQL internals don't leak to a token holder. Actionable domain
  errors (version conflict, not-found, supersede) and input-validation errors
  still pass through.
- **HSTS** (`max-age=63072000; includeSubDomains`) on public HTTPS deployments
  (gated on `PERSISTOR_PUBLIC_URL` being `https://`; the tailnet `http://` bind
  is exempt). No `preload` — that's a near-irreversible commitment, left opt-in.

## Decisions (config, not code)

> **Disposition (2026-06-23):** #1 **ACCEPTED** — rely on OIDC as the gate (it
> blocks direct-origin `/mcp`) plus the new app-layer limits; revisit if traffic
> or abuse grows. #6 **LEFT OPEN** on the Stytch Test project — reconsider before
> any public/Live launch. Both are conscious choices, not oversights.
>
> **Update (2026-07-01):** #1 re-opened for the sensitive-data import
> (PRODUCTION-READINESS plan, item S1). The app half is now SHIPPED:
> `PERSISTOR_ORIGIN_SECRET` enables the origin-lock middleware (403 without the
> matching `X-Origin-Secret` on every route but `/healthz`) and switches the
> per-IP limiters to `CF-Connecting-IP` — behind the proxy they otherwise
> collapse into one global bucket keyed on Cloudflare's own address. Remaining
> half (S1b, Brian): create the Cloudflare Transform Rule injecting the header
> on proxied traffic, then set the same value as an encrypted app secret and
> verify direct-to-origin requests 403.

### #1 — Cloudflare WAF bypass (MEDIUM-HIGH)

The bare App Platform origin (`persistor-3cbbb.ondigitalocean.app`) is reachable
directly, skipping Cloudflare's WAF/DDoS/rate-limiting. The app's OIDC auth still
gates `/mcp` direct-to-origin (so this is **not** a data-confidentiality breach),
but the edge protection is optional for an attacker who finds the origin host
(it's in certificate-transparency logs).

Options:
1. **Origin lock via shared secret (recommended).** Add a Cloudflare Transform
   Rule that injects a secret request header on proxied traffic, and have the app
   reject requests lacking it (a small middleware reading e.g.
   `PERSISTOR_ORIGIN_SECRET`). This makes Cloudflare mandatory. Requires a
   coordinated app change + CF rule — deferred pending this decision.
2. **Cloudflare Authenticated Origin Pulls (mTLS).** Stronger, but App Platform
   must validate the client cert — verify support before committing.
3. **Accept it.** Treat OIDC as the real gate (it is) and rely on the new
   app-layer limits; add Cloudflare rate-limiting rules for what edge protection
   remains useful for proxied traffic.

### #6 — Open tenant auto-provisioning (LOW)

Any valid token from the configured Stytch project auto-provisions a tenant. New
principals land in their **own empty** tenant (existing data stays isolated), so
this is abuse/resource exposure, not a breach. If the deployment is meant to be
single-user/invite-only, lock the **Stytch project to invite-only** (a Stytch
dashboard setting), or add a subject allowlist in `identity` provisioning.

## Lower-priority follow-ups (not yet done)

- Namespace charset validation at the write boundary (slug pattern), matching the
  note-id validation. Data-hygiene within a tenant; no cross-tenant/injection
  impact.
- CLI `import` should run the same note-id charset check the MCP path does.
- Optional: assert a `typ`/token-use claim on access tokens (today token-type
  separation relies on the Stytch audience invariant).
