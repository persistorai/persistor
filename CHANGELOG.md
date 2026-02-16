# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
