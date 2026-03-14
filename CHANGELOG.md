# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] — 2026-03-14

### Features

- **Temporal edges** — edges now support date-bounded relationships
  - `date_start` / `date_end` fields with EDTF date format support (exact, month, year, approximate, decades, unknown)
  - Server-computed `date_lower` / `date_upper` bounds for range queries
  - `is_current` flag for marking active/ongoing relationships
  - `date_qualifier` for storing parsed date metadata
- EDTF date parser (`internal/edtf/`) — parses Extended Date/Time Format strings and computes lower/upper time bounds
- API: edge create/update endpoints accept and return temporal fields
- CLI: `--date-start`, `--date-end`, `--current` flags on `edge create` and `edge update`
- CLI: `edge list --active-on <date>` — query edges active at a point in time
- CLI: `edge list --current` — filter to currently-active edges
- Go client updated with temporal field support
- Ingest extractor prompt updated to extract temporal relationship data from text

### Database

- Migration 012: adds `date_start`, `date_end`, `date_lower`, `date_upper`, `is_current`, `date_qualifier` columns to `kg_edges`

---

## [0.8.0] — 2026-03-14

### Features

- **Temporal edges** — edges now support time-bounded relationships
  - `date_start` and `date_end` fields (EDTF format: exact, month, year, approximate, decades)
  - `is_current` flag for ongoing relationships
  - Server-computed `date_lower`, `date_upper`, and `date_qualifier` bounds for querying
- **EDTF date parser** (`internal/edtf/`) — Extended Date/Time Format parser with bounds calculation
  - Supports: exact dates (`2019-10-15`), month precision (`2009-05`), year (`1983`), approximate (`~1983`), decades (`199X`), unknown (`..`)
- **Temporal queries** in CLI:
  - `persistor edge list --active-on 2019-10-15` — find edges active at a point in time
  - `persistor edge list --current` — find all current/ongoing relationships
  - `persistor edge create/update --date-start/--date-end/--current` flags
- **Temporal extraction in ingest** — LLM extractor now pulls date information from text during `persistor ingest`
- GraphQL schema updated with temporal edge fields
- Go client (`client/`) updated with temporal field support

### Technical

- Migration 012: adds `date_start`, `date_end`, `date_lower`, `date_upper`, `is_current`, `date_qualifier` columns to `kg_edges`
- Temporal fields are plain columns (not encrypted) for queryability
- Edge store refactored: `edge_patch.go`, `edge_temporal.go` split out for maintainability
- 32 files changed, +1,270 lines across migration, parser, model, store, API, GraphQL, CLI, client, and ingest
- 238 new EDTF parser tests, 118 new extractor tests, expanded CLI/format tests

## [0.7.0] — 2026-02-16

Initial public release.

### Features

- Knowledge graph with nodes, edges, and rich properties
- REST API (`/api/v1`) with full CRUD for nodes, edges, graph traversal, and search
- GraphQL API with playground (coexists with REST)
- WebSocket real-time change notifications (PG LISTEN/NOTIFY)
- Semantic search via pgvector embeddings (Qwen3-Embedding-0.6B)
- Hybrid search (full-text + vector with RRF fusion)
- Graph traversal: neighbors, BFS traverse, context, shortest path
- Salience scoring with boost, supersede, and recalc
- Bulk upsert for nodes and edges
- Property history tracking with timestamps and reasons
- Queryable audit log with entity/action/actor filtering
- AES-256-GCM encryption at rest (mandatory, KeyProvider interface)
- Row-Level Security multi-tenancy (PostgreSQL RLS)
- API key authentication with brute force protection
- Rate limiting (configurable per-IP)
- Prometheus metrics endpoint
- Security headers, CORS, body size limits
- Go SDK (`client/` package)
- CLI (`cmd/persistor-cli/`)
- systemd service file
- 87 unit tests

### Technical Stack

- Go 1.25 + Gin + pgx v5
- PostgreSQL 18 + pgvector
- gqlgen for GraphQL
- coder/websocket for real-time
- HashiCorp Vault for secrets management
