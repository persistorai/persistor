# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
