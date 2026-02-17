# AGENTS.md

Persistor — persistent knowledge graph + vector memory for AI agents.
Backed by PostgreSQL 18 + pgvector. Go + Gin REST API with real-time WebSocket push.

## Architecture

- `cmd/server/` — Entry point, server lifecycle, graceful shutdown
- `cmd/persistor-cli/` — CLI tool (admin, doctor, node/edge/graph/search/salience CRUD, import, init)
- `internal/api/` — Gin HTTP handlers (router, nodes, edges, search, graph, bulk, salience, health, audit, stats, history, admin, errors)
- `internal/config/` — Env-driven config with per-concern validators
- `internal/crypto/` — AES-256-GCM encryption providers (static key, Vault)
- `internal/db/` — Migrations (goose embedded), LISTEN/NOTIFY, vector dimension management
- `internal/db/migrations/` — SQL schema files (001–007: initial, property_history, audit_log, drop_old, force_rls, embed_worker_index, edge_indexes)
- `internal/dbpool/` — pgx v5 connection pool
- `internal/graphql/` — gqlgen schema, resolvers, type conversion, middleware, context helpers
- `internal/httputil/` — Shared HTTP response helpers
- `internal/metrics/` — Prometheus metrics (request duration, embed worker, WebSocket)
- `internal/middleware/` — Auth (API key + caching), rate limiting, brute force protection, body limits, request IDs, security headers, Prometheus middleware
- `internal/models/` — Node, Edge, Search, Salience, PropertyHistory types + validation
- `internal/security/` — Brute force detection (shared with middleware)
- `internal/service/` — NodeService, SearchService, BulkService, EmbedWorker, AuditWorker, EmbeddingService
- `internal/store/` — 27 files across focused stores (Node, Edge, Search, Graph, Bulk, Salience, Embedding, History, Audit, Tenant) + helpers (encrypt, scan, bulk_helpers)
- `internal/ws/` — WebSocket hub, client, event buffer, event types
- `scripts/` — Backup, restore, health-check, SQLite migration, git hooks

## Tech Stack

| Component  | Choice                          | Notes                              |
| ---------- | ------------------------------- | ---------------------------------- |
| Language   | Go 1.25                         | `/usr/local/go/bin/go`             |
| HTTP       | Gin (`gin-gonic/gin`)           | Fast, lightweight HTTP framework   |
| CORS       | `gin-contrib/cors`              | Configured in router.go            |
| Logging    | Logrus (`sirupsen/logrus`)      | JSON formatter, structured logging |
| WebSocket  | `github.com/coder/websocket`    | Context-aware, successor to nhooyr |
| Database   | PostgreSQL 18 + pgvector        | Native install, not Docker         |
| DB Driver  | pgx v5 (`jackc/pgx`)            | Native LISTEN/NOTIFY, pool         |
| Embeddings | Qwen3-Embedding-0.6B via Ollama | 1024d vectors, localhost:11434     |
| GraphQL    | gqlgen (`99designs/gqlgen`)     | Coexists with REST at /graphql     |
| Metrics    | Prometheus (`client_golang`)    | /metrics endpoint                  |
| Migrations | goose v3 (`pressly/goose/v3`)   | Embedded SQL, up/down rollback     |
| Linting    | golangci-lint + markdownlint    | Pre-commit hook enforces both      |

## Key Commands

```bash
# Go toolchain (not on default PATH in some contexts)
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

# Build
make build                    # Binary → bin/server

# Run
make run                      # Build + start on :3030

# Lint (Go)
make lint                     # golangci-lint run ./...
make lint-fix                 # golangci-lint run --fix ./...

# Lint (Markdown)
make lint-md                  # markdownlint '**/*.md'

# Full CI
make ci                       # format + vet + lint + lint-md + test + coverage

# Test
make test                     # go test -v -race ./...

# Install git hooks after clone
make setup-hooks
```

## Project Structure

```text
persistor/
├── cmd/
│   ├── server/main.go           # Entry point
│   └── persistor-cli/           # CLI (admin, doctor, node/edge/graph/search/salience, import, init)
├── client/                      # Go client library (nodes, edges, search, graph, bulk, salience, audit, admin)
├── internal/
│   ├── api/                     # Gin handlers (router, nodes, edges, search, graph, bulk, salience, health, audit, stats, history, admin)
│   ├── config/                  # Env-driven config with per-concern validators
│   ├── crypto/                  # AES-256-GCM encryption (static + Vault providers)
│   ├── db/                      # Migrations (goose, embedded), LISTEN/NOTIFY, vector dims
│   │   └── migrations/          # 001–007 (initial, property_history, audit_log, drop_old, force_rls, embed_worker_index, edge_indexes)
│   ├── dbpool/                  # pgx v5 connection pool
│   ├── graphql/                 # gqlgen schema, resolvers, type conversion, middleware
│   ├── httputil/                # Shared HTTP response helpers
│   ├── metrics/                 # Prometheus instrumentation
│   ├── middleware/              # Auth (API key + cache), rate limiter, brute force, body limit, request ID, security headers, Prometheus
│   ├── models/                  # Node, Edge, Search, Salience, PropertyHistory types + validation
│   ├── security/                # Brute force detection
│   ├── service/                 # NodeService, SearchService, BulkService, EmbedWorker, AuditWorker, EmbeddingService
│   ├── store/                   # 27 files: Node, Edge, Search, Graph, Bulk, Salience, Embedding, History, Audit, Tenant + helpers
│   └── ws/                      # WebSocket hub, client, event buffer, event types
├── scripts/
│   ├── hooks/pre-commit         # Go lint + markdown lint
│   ├── migrate/                 # One-time SQLite → Postgres migration
│   ├── backup.sh                # pg_dump + verify + encrypt + upload
│   ├── restore.sh               # Decrypt + restore + verify
│   └── health-check.sh          # HTTP health probe
├── VERSION                      # Semver (0.7.0)
├── CHANGELOG.md
├── Makefile
├── openapi.yaml                 # OpenAPI spec
├── .golangci.yml                # Strict linter config
├── .markdownlint.yaml
└── .gitattributes
```

## Configuration (Environment Variables)

| Variable          | Default                                                                    |
| ----------------- | -------------------------------------------------------------------------- |
| `DATABASE_URL`    | `postgres://persistor:<password>@localhost:5432/persistor?sslmode=disable` |
| `PORT`            | `3030`                                                                     |
| `LISTEN_HOST`     | `127.0.0.1`                                                                |
| `CORS_ORIGINS`    | `http://localhost:3002`                                                    |
| `OLLAMA_URL`      | `http://localhost:11434`                                                   |
| `EMBEDDING_MODEL` | `qwen3-embedding:0.6b`                                                     |

## Database Schema

See `internal/db/migrations/001_initial.sql` for full schema. Key points:

- **Tenant isolation:** RLS on every table via `tenant_id` + `current_setting('app.tenant_id')`
- **Nodes:** `kg_nodes` — id, type, label, properties (JSONB), embedding (vector(1024)), salience tracking
- **Edges:** `kg_edges` — source→target with relation, weight, JSONB properties, salience
- **Triggers:** `updated_at` auto-update, `pg_notify('kg_changes', ...)` on all writes
- **Indexes:** GIN for full-text search, IVFFlat for vector similarity, B-tree on type/salience/timestamps

## Coding Standards

**Go patterns:**

- `internal/` package layout, no `pkg/`
- `fmt.Errorf("context: %w", err)` for error wrapping
- Check ALL errors — errcheck is enabled and strict
- Constructor injection for dependencies (logger, pool, hub)
- Gin handler signature: `func (h *Handler) Method(c *gin.Context)`
- JSON responses via `c.JSON(status, gin.H{...})` or typed structs
- Logrus structured logging: `log.WithFields(logrus.Fields{...}).Info("msg")`

**Import ordering (enforced by gci):**

1. Standard library
2. Third-party packages
3. Local packages (`github.com/persistorai/persistor/...`)

**Commits:** Conventional commits (`feat:`, `fix:`, `chore:`, `refactor:`).

**After code changes, always run:** `make lint` then `make build`

## Current State (v0.7.0)

Production-ready with clean architecture:

- **Repository split:** 27 files across focused stores in `internal/store/`
- **Service layer:** Handler → Service → Store separation, with AuditWorker background processing
- **Audit log:** Tenant-isolated `kg_audit_log` with query/purge endpoints
- **Auth & security:** API key auth with caching, brute force protection, body limits, request IDs
- **Go client library:** `client/` package for programmatic access (nodes, edges, search, graph, bulk, salience, audit)
- **CLI tool:** `cmd/persistor-cli/` for admin, diagnostics, and CRUD operations
- **Prometheus metrics:** `sms_*` metrics, `/metrics` endpoint, Gin middleware
- **WebSocket hardening:** Ping/pong, monotonic event IDs, reconnection replay,
  event buffering, permessage-deflate, graceful drain on shutdown
- **GraphQL API:** gqlgen at `/api/v1/graphql` + playground (coexists with REST)
- **Migrations:** goose v3, 7 SQL schema files, embedded in binary
- **Semver:** VERSION file, git tags, CHANGELOG.md
- **Systemd:** Deployed via `persistor.service` with EnvironmentFile
- **OpenAPI:** `openapi.yaml` spec for REST API documentation

## Planning Doc

See planning docs for architecture decisions, schema design, migration plan, backup strategy, and phased task breakdown.

## Important Notes

- PostgreSQL 18 is installed and running — native install, not Docker
- Service listens on localhost only (127.0.0.1:3030) — never expose externally
- Backup data is age-encrypted — never store plaintext exports
- The `migrate-from-sqlite` binary in .gitignore — only the source (`scripts/migrate-from-sqlite.go`) is tracked
- Go binary path: `/usr/local/go/bin/go` (may not be on PATH in hooks/scripts)
- golangci-lint path: `~/go/bin/golangci-lint`

## Code Architecture Rules

**These rules prevent monolithic "God Object" patterns that degrade code quality and agent effectiveness.**

- **One struct per concern.** Don't add methods to an existing struct when the concern
  is different. Create a new file + struct.
- **No file over 300 lines.** Split before 300. Over 500 is a bug.
- **Define interfaces before implementation.** Small, focused interfaces
  (`NodeReader`, `NodeWriter`) beat splitting a large one later.
- **Each package file gets its own `_test.go`.** Tests live next to the code they test.
- **Handlers never import storage directly.** Depend on interfaces, not concrete types.
- **Shared helpers go in dedicated files.** Don't bury utilities inside large files —
  extract them (`encrypt.go`, `notify.go`, `response.go`).
- **One PR per concern.** Small, focused changes are easier to review.
- **After every feature, ask: "what should we refactor?"** Building reveals pain points.
