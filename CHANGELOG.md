# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.10.1] — 2026-07-01

Reliability & disaster recovery (production-readiness Phase 2).

### Added

- Bounded DB retry/backoff at daemon boot (~90s, SIGTERM-aware) on the new
  `dbpool.ErrDBUnreachable` — a transient DB blip at container start no longer
  kills the process; config/posture errors (bad URL, RLS-bypassing role) still
  fail fast.
- Nightly encrypted off-site backup (`scripts/backup-prod.sh` + systemd user
  timer in `deploy/backup/`): per-tenant `persistor export` pulled off
  DigitalOcean, age-encrypted, 14 kept; restore runbook in
  `deploy/do/README.md`. Live-verified with a prod canary round-trip.
- Tests: `buildAuth` wiring, new config flags, and the SUPERUSER/BYPASSRLS
  boot-refusal path.

## [0.10.0] — 2026-07-01

Security-hardening phase 1 of the production-readiness plan, gating the
sensitive-data import (`planning-docs/persistor/PRODUCTION-READINESS.md`).

### Added

- **Origin lock** (`PERSISTOR_ORIGIN_SECRET`): when set, every route except
  `/healthz` requires the matching `X-Origin-Secret` header (injected by a
  Cloudflare Transform Rule on proxied traffic), making the edge WAF mandatory
  instead of bypassable via the bare origin host. With the lock enforced, the
  per-IP limiters key on `CF-Connecting-IP` instead of the proxy address —
  previously they collapsed into one global bucket behind Cloudflare.
- **DCR relay controls**: `PERSISTOR_DCR_ENABLED=false` shuts off the
  unauthenticated `/register` relay (and drops `registration_endpoint` from the
  AS metadata) as an abuse response; every registration attempt is logged.
- **`/metrics` tenant allowlist** (`PERSISTOR_METRICS_TENANTS`): restricts the
  endpoint beyond "any valid token", which under open auto-provisioning
  includes self-provisioned strangers.
- **Access-token type assertion** in OIDC verify: rejects ID-token markers
  (`at_hash`/`nonce` claims) and non-access `typ` headers, so token-type
  separation no longer rests solely on the Stytch audience invariant.
- **Namespace validation** at the write boundary (lowercase slug, no `:`) and
  the same note-id charset check on CLI `import` frontmatter ids.
- **govulncheck** CI job + `make vulncheck`; `make gate` / `make deploy`
  one-command build gate and production deploy (`scripts/gate.sh`,
  `scripts/deploy.sh`).

### Changed

- CORS allow headers are granted only to `PERSISTOR_TRUSTED_ORIGINS` (default
  claude.ai) instead of reflecting any Origin.

## [0.9.2] — 2026-06-23

Production security hardening from the post-deploy review of `mcp.persistor.ai`
(written retroactively 2026-07-01; shipped as commit 665b68e, tag v0.9.2).

### Changed

- `/metrics` is bearer-gated (was public — it exposed DB pool capacity on a
  public ingress).
- `memory_search` / `brief` result limits capped at 500, mirroring
  `memory_list`; closes a one-request DoS via `LIMIT 1e9`.
- `/mcp` gains a coarse pre-auth per-IP backstop and a 120s `WriteTimeout`.
- Tenant derivation rejects a token subject containing the `|` join separator
  (forecloses a future multi-issuer tenant collision; existing ids unchanged).
- Store/DB errors are replaced with a generic message at the tool boundary;
  actionable domain errors still pass through.
- HSTS on public HTTPS deployments.

## [0.9.1] — 2026-06-22

Browser MCP client support (retroactive entry; commit 25758e8, tag v0.9.1 was
never pushed — the image tag existed on DOCR only).

### Fixed

- claude.ai web connectors: CORS preflight handling + trusted browser origins
  on the CSRF guard (`PERSISTOR_TRUSTED_ORIGINS`, default `https://claude.ai`).
  Pre-fix, the preflight got 401 and the POST 403 — "Couldn't connect".

## [0.9.0] — 2026-06-22

Pre-production hardening pass (from a multi-dimension code review), ahead of
standing up the hosted environment.

### Added

- `persistor migrate` applies schema migrations explicitly, for the production
  split-role posture (migrate as the owner, run the daemon as a non-owner role).
- `PERSISTOR_AUTO_MIGRATE` (default true): off = the daemon does not migrate at
  boot, refuses a stale schema, and asserts it is not the table owner.
- `/metrics` endpoint (request counts by status, duration, 429s, DB pool
  saturation) and an `X-Request-Id` correlation id threaded into the access log
  with tenant attribution. `PERSISTOR_LOG_LEVEL` configures verbosity.
- `PERSISTOR_DB_MAX_CONNS` makes the pool size configurable.
- Read-path per-tenant rate limiting; request-body cap on `/mcp`; per-IP limit on
  the `/register` proxy. `deploy/sql/provision-roles.sql` documents the two-role
  provisioning.

### Changed

- Migration 010: tenant-scoped composite FTS index (btree_gin), `supersedes`
  index, predicate-aligned partial indexes, a `chunks` unique constraint, a
  1 MiB note-body CHECK, and autovacuum tuning. Migration 011 indexes
  `(tenant_id, namespace, id)` for namespace-filtered listing.
- Supersession reconcile is scoped to the affected notes and folded into the
  write/delete/restore transaction (atomic; no TOCTOU on the supersedes target).
- The auth hot path no longer writes `last_seen_at` on every request (was a write
  + row lock per read); it refreshes only when stale.
- Rate-limiter buckets are evicted when idle (bounded memory under many tenants).
- Docs realigned to the Postgres-native, OIDC remote-MCP design (README, env
  examples, systemd unit); oversized files split to the ≤300-line standard.

## [0.1.0] — 2026-06-18

Memory for AI agents: the agent's markdown notes are the source of truth; a
PostgreSQL full-text index over them makes them searchable.

### Added

- Index (`internal/index`): a content-hash file-sync indexer, markdown frontmatter
  parsing, heading-aware chunking, and FTS retrieval (`SearchNotes`) over Postgres
  `tsvector`.
- Dynamic working-set (`persistor brief`): every pinned Core note plus
  token-budgeted Tail relevant to the current project; degrades to
  Core-read-from-disk when the index is unreachable.
- Consolidation write path (`persistor consolidate`): deterministic plan apply
  that writes prose notes and reconciles supersession; a write whose `supersedes`
  resolves to its own note is rejected.
- Retrieval eval (`internal/eval`): deterministic recall@k against baselines
  (full retrieval / static `MEMORY.md` / no memory), gating every change.
- CLI: `persistor reindex`, `brief`, `search`, `consolidate`, `eval`.
- MCP server (`cmd/persistor-mcp`, stdio): `memory_search`, `memory_get`,
  `memory_write`, `brief`; plus Claude Code hooks and the `/consolidate` skill
  under `harness/claude-code/`.
- Multi-tenant isolation via PostgreSQL row-level security.
