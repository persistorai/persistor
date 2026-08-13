# Persistor

Memory for AI agents. Durable prose notes plus a PostgreSQL full-text index over
them. The note row in Postgres is the source of truth — versioned, with
append-only history. Index everything; decide relevance at query time. No entity
extraction, no graph, no vectors — notes in, ranked notes out.

## Status

Built and run in production during 2026, in two topologies: hosted (DigitalOcean
App Platform behind Cloudflare) and local (a workstation Postgres cluster served
to MCP clients over a Tailscale tailnet via `tailscale serve`). The hosted
service was retired in July 2026 after a market assessment; the codebase is
published as a reference implementation of a multi-tenant, identity-aware MCP
memory server. It builds, the test suite runs (`make gate`), and the design
decisions are documented throughout `deploy/`.

## How it works

An agent already writes correct prose it authored and reads natively. Persistor
stores those notes in Postgres and keeps a full-text index (a `tsvector` chunk
projection) beside them for ranked retrieval. Every write is versioned: an
append-only `note_versions` row records each create/update/delete/restore, and a
database trigger blocks tampering. Agents reach Persistor only through the MCP
daemon — there is no filesystem note store and no stdio binary.

Multi-tenant by PostgreSQL row-level security: every row is scoped to a tenant,
and the tenant is derived from the verified OIDC token (`tenant =
uuidv5(issuer | subject)`), so the same login is the same tenant on every device.

## What's here

- `internal/index` — the engine: PG-native versioned notes, heading-aware
  chunking, FTS retrieval (`SearchNotes`), the bounded working-set (`brief`),
  the supersession write path, and import/export.
- `internal/mcpengine` — the transport-agnostic MCP engine, the eight tool
  schemas, and the per-tenant write rate limiter.
- `internal/mcpauth` — the OIDC/JWT (JWKS) bearer verifier; derives the stable
  tenant from `iss|sub`.
- `internal/identity` — the RLS-exempt tenant + identity onboarding store
  (resolve/provision per login).
- `internal/eval` — a deterministic retrieval eval (recall@k) that gates every
  change against baselines.
- `internal/db`, `internal/dbpool`, `internal/config` — the goose migrations +
  runner, the connection pool, and the build-time version.
- `cmd/persistor-cli` — the `persistor` operator CLI.
- `cmd/persistor-server` — the remote MCP HTTP daemon (OIDC).
- `harness/claude-code` — install artifacts for wiring Persistor into Claude Code.

## MCP server

`persistor-server` is the only client surface: a remote MCP daemon over
Streamable HTTP, authenticated by OIDC. It serves eight tools:

- `memory_search` — FTS retrieval over the tenant's notes.
- `memory_get` — fetch a single note by id.
- `memory_list` — browse/page note summaries, optionally filtered by namespace.
- `memory_namespaces` — list namespaces and their counts.
- `memory_write` — create or update a note (with optional supersession);
  searchable with no indexing lag.
- `memory_delete` / `memory_restore` — tombstone a note out of retrieval and
  undo that (history preserved; hard removal is the operator-level `purge`).
- `brief` — assemble the working-set: every pinned Core note plus the Tail notes
  most relevant to a seed, under a token budget.

Local and remote clients alike are normal MCP clients of the (local or remote)
server.

## CLI

`persistor` is the operator tool (DB-direct, not the user path):

- `persistor brief` — assemble and print the working-set.
- `persistor search <query>` — run FTS retrieval and print ranked notes.
  Excludes superseded notes by default; `--include-superseded` shows history.
- `persistor list` / `persistor namespaces` — browse notes and namespaces.
- `persistor eval --fixture <f>` — score the index against the baselines
  (full retrieval vs static `MEMORY.md` vs no memory).
- `persistor import` / `persistor export` — bulk-load `.md` into a
  tenant/namespace, or dump a tenant's notes.
- `persistor admin` — tenant + identity administration.
- `persistor purge` / `persistor delete-tenant` — hard-delete a single note, or
  an entire tenant (data portability + account deletion).

## Quick start

The daemon needs only a Postgres connection and its OIDC settings. Connect as a
`NOSUPERUSER NOBYPASSRLS` role that does **not** own the schema — RLS is the
tenant boundary, and the pool refuses to start otherwise.

```bash
# 1. Provision roles: a migrator (owns the schema) and the app role (least-privilege).
createdb persistor
psql persistor <<'SQL'
CREATE ROLE persistor_migrator LOGIN PASSWORD '...';
CREATE ROLE persistor_app      LOGIN PASSWORD '...' NOSUPERUSER NOBYPASSRLS;
ALTER SCHEMA public OWNER TO persistor_migrator;
SQL

# 2. Apply migrations as the migrator (owner).
export DATABASE_URL=postgres://persistor_migrator:...@localhost:5432/persistor?sslmode=disable
persistor migrate

# 3. Run the daemon as the non-owner app role.
cp deploy/persistor-server.env.example ~/.persistor/server.env   # fill in DATABASE_URL (app role) + OIDC
persistor-server
```

See `deploy/persistor-server.env.example` for the full daemon configuration.

## Schema

All tenant tables are RLS-isolated (`ENABLE` + `FORCE`), scoped to
`current_setting('app.tenant_id')`:

- `notes(id, kind, tier, namespace, title, body, supersedes, superseded,
  deleted, version)` — the source of truth.
- `note_versions(note_id, op, version, surface, …)` — append-only audit log;
  a trigger blocks UPDATE/DELETE/TRUNCATE except an explicit operator purge.
- `chunks(note_id, ord, text, search_tsv)` — the FTS unit.

Two RLS-exempt routing tables hold no note content and are consulted before the
tenant is known:

- `tenants(id, …)` — one row per tenant.
- `identities(issuer, subject, tenant_id, role, last_seen_at)` — login → tenant
  routing.

## Security model

The part this codebase cares most about: tenancy that survives an
application-layer bug.

- Tenant isolation is enforced by PostgreSQL row-level security (`ENABLE` +
  `FORCE`), not by application code. The daemon connects as a `NOSUPERUSER
  NOBYPASSRLS` role that does not own the schema, and the connection pool
  asserts this at boot and refuses to start otherwise.
- The tenant is derived from the verified OIDC token (`uuidv5(issuer |
  subject)`), never from client input.
- `note_versions` is append-only, with a database trigger blocking
  UPDATE/DELETE/TRUNCATE outside an explicit operator purge.
- Per-tenant write rate limiting in the MCP engine.
- `make gate` runs the full self-provisioned test suite, including the RLS
  boundary tests and a deterministic retrieval eval gated against baselines.

## License

Proprietary and confidential. Copyright (c) Brian Colinger. All rights reserved.
See [LICENSE](LICENSE).
