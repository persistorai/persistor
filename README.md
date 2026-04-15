<p align="center">
  <img src=".github/logo.png" alt="Persistor" width="120" height="120" style="border-radius: 24px;">
</p>

<h1 align="center">Persistor</h1>

<p align="center">
  <strong>Persistent knowledge graph + vector memory for AI agents</strong><br>
  <a href="https://persistor.ai">Website</a> · <a href="INTEGRATION.md">API Docs</a> · <a href="CHANGELOG.md">Changelog</a>
</p>

<p align="center">
  <img alt="Version" src="https://img.shields.io/badge/version-0.6.0-green">
  <img alt="License" src="https://img.shields.io/badge/license-AGPL--3.0-blue">
  <img alt="Go" src="https://img.shields.io/badge/go-1.25+-00ADD8?logo=go&logoColor=white">
  <img alt="PostgreSQL" src="https://img.shields.io/badge/PostgreSQL-16+-336791?logo=postgresql&logoColor=white">
</p>

Durable, searchable, graph-structured memory across sessions. Designed for AI agents
and the developers who build them.

## Architecture

- **Go + Gin** — Fast, type-safe REST API
- **PostgreSQL 16+ + pgvector** — Knowledge graph storage with vector similarity search
- **Hybrid search** — Reciprocal Rank Fusion combines full-text + vector results; falls back to text-only if embeddings are unavailable
- **Salience scoring** — Every node tracks access patterns, recency, and user boosts to surface the most relevant memories automatically
- **AES-256-GCM encryption** — All node/edge properties encrypted at rest, transparent to API consumers
- **Ollama embeddings** — Automatic vector generation (qwen3-embedding:0.6b)
- **Row-Level Security** — Complete tenant isolation; one API key = one tenant
- **WebSocket** — Real-time change notifications via PostgreSQL LISTEN/NOTIFY

## CLI

The `persistor` CLI is a first-class interface for interacting with your knowledge graph
without writing a single HTTP request.

```bash
# Install
make build-cli && sudo make install-cli   # installs to /usr/local/bin/persistor

# Configure (stored in ~/.persistor/config.yaml)
persistor init
```

**Key commands:**

```bash
# Nodes
persistor node create --type person --label "Alice Smith" --id alice
persistor node get alice
persistor node list --type person --min-salience 0.5

# Search
persistor search "active projects"           # full-text
persistor search --semantic "project risks"  # vector similarity
persistor search --hybrid "database memory"  # text + vector (recommended)

# Graph traversal
persistor graph neighbors alice
persistor graph traverse alice --hops 3
persistor graph context alice              # node + neighbors + edges in one call

# Salience
persistor salience boost alice             # mark a node as important
persistor salience recalc                  # recompute scores from access patterns

# Admin & diagnostics
persistor admin stats                      # knowledge graph statistics
persistor admin reprocess-nodes --search-text --embeddings
persistor admin maintenance-run --refresh-search-text --scan-stale-facts
persistor admin merge-suggestions --type person --min-score 0.7
persistor doctor                           # check server connectivity and config
```

**Global flags:** `--url` (default `http://localhost:3030`, or `PERSISTOR_URL`),
`--api-key` (or `PERSISTOR_API_KEY`), `--format json|table|quiet`.

## Salience Scoring

Every node and edge carries a `salience_score` that Persistor updates automatically:

- **Access patterns** — nodes read frequently score higher
- **Recency** — recently accessed nodes decay more slowly
- **User boosts** — explicit `salience/boost` marks a node as important (`user_boosted: true`)
- **Supersession** — outdated nodes link to their replacement via `superseded_by`
- **Recalc** — `POST /salience/recalc` (or `persistor salience recalc`) refreshes all scores

Query by minimum salience (`?min_salience=0.5`) to retrieve only what matters right now.

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 16+ with pgvector extension
- Ollama running locally (for embedding generation)

### Setup

```bash
git clone https://github.com/persistorai/persistor.git
cd persistor

# Configure environment (or use systemd EnvironmentFile — see Deployment)
export DATABASE_URL="postgres://persistor:<password>@localhost:5432/persistor?sslmode=disable"
export ENCRYPTION_PROVIDER=static
export ENCRYPTION_KEY="<64-hex-char-key>"  # 32 bytes, hex-encoded
export PORT=3030

# Build and run (migrations run automatically on startup)
make build
make run
```

### Verify

```bash
curl http://localhost:3030/api/v1/health
# {"status": "ok"}

curl http://localhost:3030/api/v1/ready
# {"status":"ok","checks":{"database":"ok","schema":"ok","ollama":"ok"}}
```

## Configuration

| Variable              | Default                  | Description                                     |
| --------------------- | ------------------------ | ----------------------------------------------- |
| `DATABASE_URL`        | — (required)             | PostgreSQL connection string                    |
| `PORT`                | `3030`                   | HTTP listen port                                |
| `LISTEN_HOST`         | `127.0.0.1`              | Listen address (must be loopback)               |
| `CORS_ORIGINS`        | `http://localhost:3002`  | Comma-separated allowed origins                 |
| `OLLAMA_URL`          | `http://localhost:11434` | Ollama API endpoint (must be localhost)         |
| `OLLAMA_MODEL`        | `gemma4:e4b`             | Default Ollama chat/extraction model            |
| `EMBEDDING_MODEL`     | `qwen3-embedding:0.6b`   | Embedding model name                            |
| `LOG_LEVEL`           | `info`                   | Log level                                       |
| `ENCRYPTION_PROVIDER` | `static`                 | `static` (env key) or `vault` (HashiCorp Vault) |
| `ENCRYPTION_KEY`      | — (required if static)   | 64 hex chars (32-byte AES key)                  |
| `VAULT_ADDR`          | `http://127.0.0.1:8200`  | Vault address (if provider=vault)               |
| `VAULT_TOKEN`         | — (required if vault)    | Vault token                                     |

## API Documentation

See **[INTEGRATION.md](./INTEGRATION.md)** for the complete API reference
with curl examples, data model documentation, and agent-specific usage patterns.

### Endpoint Summary

| Group     | Endpoints                                                                                                    |
| --------- | ------------------------------------------------------------------------------------------------------------ |
| Health    | `GET /health`, `GET /ready`                                                                                  |
| Nodes     | `GET/POST /nodes`, `GET/PUT/PATCH/DELETE /nodes/:id`                                                         |
| Edges     | `GET/POST /edges`, `PUT/PATCH/DELETE /edges/:source/:target/:relation`                                       |
| Search    | `GET /search`, `GET /search/semantic`, `GET /search/hybrid` (label + alias-aware retrieval)                 |
| Graph     | `GET /graph/neighbors/:id`, `GET /graph/traverse/:id`, `GET /graph/context/:id`, `GET /graph/path/:from/:to` |
| Bulk      | `POST /bulk/nodes`, `POST /bulk/edges`                                                                       |
| Salience  | `POST /salience/boost/:id`, `POST /salience/supersede`, `POST /salience/recalc`                              |
| WebSocket | `GET /ws`                                                                                                    |
| Admin     | `GET /stats`, `POST /admin/backfill-embeddings`, `POST /admin/reprocess-nodes`, `POST /admin/maintenance/run`, `GET /admin/merge-suggestions`, `POST/GET /admin/retrieval-feedback` |
| Audit     | `GET /audit`, `DELETE /audit`                                                                                |
| History   | `GET /nodes/:id/history`                                                                                     |
| Metrics   | `GET /metrics` (Prometheus, outside `/api/v1/`)                                                              |
| GraphQL   | `POST /graphql`, `GET /graphql/playground`                                                                   |

All under `/api/v1/` unless noted.

## Development

```bash
make build          # Build server and CLI binaries
make run            # Build and run the server
make test           # Run tests with race detection
make test-coverage  # Tests + HTML coverage report
make lint           # golangci-lint
make lint-fix       # Auto-fix lint issues
make format         # gofmt + goimports
make ci             # Full CI: format → vet → lint → test + coverage
```

## Memory Evaluation

Persistor includes an early evaluation harness for measuring memory retrieval quality against real benchmark questions.

```bash
persistor eval run --fixture ./testdata/eval/scout-memory-baseline.json
persistor eval run --fixture ./testdata/eval/scout-memory-baseline.json --format table
persistor eval run --fixture ./testdata/eval/scout-memory-phase2.json --format table
persistor eval run --fixture ./testdata/eval/scout-memory-phase2.json --compare-rerank-profile term_focus --compare-rerank-profile salience_focus
```

The `--compare-rerank-profile` flow runs the fixture once with the default prototype rerank profile, then once per named profile, and emits a JSON comparison report. Use plain `--format table` runs when you want the per-question table for one fixture/profile at a time.

The evaluation output reports:

- pass/fail per question
- recall@k
- precision@k
- average latency
- returned hits for each prompt

Use this as a baseline before and after search or memory-quality changes.

The phase 2 fixture adds harder prompts for:

- temporal shorthand and date-based recall
- paraphrased ownership questions
- graph-aware context queries
- policy and preference retrieval

## Phase 1 Memory Quality Progress

Phase 1 currently improves the memory system in these ways:

- benchmark harness for repeatable retrieval evaluation
- richer canonical embedding text for semantic search
- expanded full-text indexing over label, type, and selected properties
- lightweight intent-aware retrieval biasing
- smarter memory-plugin result merging and deduplication

The public search and memory tool names remain stable. Improvements are shipped behind the existing interfaces rather than through version-suffixed endpoint names.

## Phase 2 Memory Operations

Phase 2 finishes the first operator-facing maintenance loop for memory quality.

- **Alias-aware retrieval**: exact lookup, full-text search, and hybrid search can all surface a node by its stored alias, not just its canonical label.
- **Alias normalization**: aliases are matched case-insensitively with whitespace normalization, so `Bill   Gates` and `bill gates` collapse to the same stored form.
- **Current API surface**: alias storage exists in the data model and is used by ingest, search, and duplicate analysis, but there is not yet a public REST or CLI alias CRUD command. Operators should treat alias creation today as an internal/system workflow, not a documented end-user API.
- **Entity resolution during ingest**: ingest tries exact ID, alias-aware exact lookup, then search-backed candidates. It auto-matches only when confidence is high enough and the best result is clearly ahead of the runner-up.
- **Practical confidence thresholds**: candidates below `0.50` are ignored, `>= 0.93` can auto-match, and near-ties within `0.08` are treated as ambiguous to avoid silent merges.
- **Duplicate suggestions**: `persistor admin merge-suggestions` lists explainable likely duplicates, ordered by score, but does not merge anything automatically.
- **Maintenance workflows**:
  - Use `persistor admin reprocess-nodes` when you want to backfill missing `search_text` and/or embeddings for existing nodes.
  - Use `persistor admin maintenance-run` when you want a broader operator scan that can refresh derived fields, count stale fact evidence, and estimate duplicate-candidate volume.
  - Reserve a future full re-ingest for extractor/schema changes that require re-reading original source material, not for routine refresh/backfill work.

## Phase 3 Episodic Memory and Recall

Phase 3 adds a bounded episodic layer alongside the existing semantic graph.

- **Semantic memory** still lives in nodes, edges, facts, aliases, salience, and search indexes.
- **Episodic memory** adds durable `episodes` and `event records` so the system can preserve compact timelines, decision points, tasks, promises, and outcomes without turning every moment into a first-class graph node.
- **Current public surface**: these foundations are implemented and used internally by ingest, recall assembly, and the OpenClaw memory plugin, but there is not yet a public REST or CLI CRUD surface for episodes, events, or recall packs.

### Episodic structures

- An **episode** is a bounded container with `title`, optional `summary`, `status` (`open` or `closed`), optional time range, and optional links back to a primary project node or source artifact node.
- An **event record** is a first-class event-like memory item linked to an episode. Supported event kinds are `observation`, `conversation`, `message`, `decision`, `task`, `promise`, and `outcome`.
- Event records can carry bounded evidence pointers, optional timestamps or ranges, a confidence score, and links back to graph nodes with explicit roles.

### Bounded ingest behavior

Current episodic ingest is intentionally conservative.

- Persistor only creates episodic records when extraction contains a strong enough event signal.
- Today that means:
  - extracted entities typed as `decision` or `event`
  - extracted entities with an explicit event kind in `event_kind`, `kind`, or `event_type`
  - extracted relationships with relation `decided`
- For each ingest source, Persistor builds at most one synthetic episode when at least one event record is created.
- Event records are deduplicated by normalized `(kind, title)` within that ingest pass.
- Dry runs do not persist episodic records, but real ingest reports include `Episodic: <episodes>, <events>` when any were created.

### Belief tracking

Belief tracking is currently a bounded internal summary derived from fact evidence on node properties.

- The current preferred value for a property is stored in the node as normal application data.
- Persistor also writes an internal `_fact_beliefs` structure that summarizes the current belief state for that property.
- Each belief contains a **preferred claim** plus zero or more competing claims, with confidence, evidence count, source list, and last-observed metadata.
- Status is:
  - `supported` when one preferred claim clearly leads
  - `contested` when the runner-up is close enough to matter
  - `superseded` when evidence exists but the property no longer has a current stored value
- This is meant for retrieval, operator review, and recall-pack assembly. It is not yet a standalone belief-management API.

### Recall packs

Recall packs are compact, deterministic summaries for one or more active node IDs.

They currently assemble:

- core entities
- notable neighbors
- recent linked episodes/events
- open decisions/tasks/promises
- contradictions from contested or superseded beliefs
- strongest supporting evidence pointers

The implementation is bounded on purpose. Each section has small defaults and a hard cap of `10` items per section.

### OpenClaw memory plugin behavior

The `memory-persistor` extension now does session-aware retrieval across file memory and the Persistor graph.

- It registers the unified `memory_search` and `memory_get` tools.
- `memory_search` builds a retrieval context from optional tool params:
  - `currentSessionEntities` (aliases: `sessionEntities`, `entities`)
  - `recentMessages` (aliases: `messages`, `recentTurns`)
  - `activeWorkContext` (aliases: `workContext`, `activeTask`, `taskContext`)
- That context is used to:
  - infer file-vs-graph source preference
  - expand the query into a few bounded query variants
  - lightly boost matching file and graph results
- `memory_get` reads file paths normally, and otherwise attempts a Persistor node lookup by UUID or other node ID/label string, optionally including graph context.
- The plugin also exposes `memory-kg status` and `memory-kg search <query>` for direct operator checks in OpenClaw.

For command examples, data model details, and plugin/operator notes, see `INTEGRATION.md`.

## Phase 4 Retrieval Tuning and Feedback

Phase 4 adds bounded operator-facing retrieval tuning surfaces without changing the default public search contract.

- **Default behavior is unchanged**: normal `GET /search/hybrid` and `persistor search --hybrid` calls keep using the existing hybrid search flow.
- **Prototype reranking is internal/optional**: the extra rerank pass is only enabled when `internal_rerank=prototype` is supplied on hybrid search requests, or when an eval fixture uses `search_mode: "hybrid_rerank"`.
- **Bounded candidate set**: the prototype reranker only reorders the already-retrieved hybrid candidates. It overfetches to `limit * 3`, capped at `50`, then trims back to the requested limit.
- **Named prototype profiles**: `default`, `term_focus`, and `salience_focus`. Unknown profile names normalize back to `default`.

### Internal rerank profiles

- `default`: balanced scoring across exact/partial term matches, search text coverage, salience, and original rank bias.
- `term_focus`: gives more weight to label/search-text term matches and query coverage, with less salience bias. Useful for wording-sensitive prompts.
- `salience_focus`: leans more on salience and existing rank order, with lighter term weighting. Useful when your graph curation and boosts already encode importance well.

### Enabling the prototype reranker

API example:

```bash
curl "http://localhost:3030/api/v1/search/hybrid?q=Persistor%20deploy%20fix&limit=5&internal_rerank=prototype&internal_rerank_profile=term_focus" \
  -H "Authorization: Bearer $API_KEY"
```

Go client example:

```go
results, err := c.Search.Hybrid(ctx, "Persistor deploy fix", &client.SearchOptions{
    Limit:                 5,
    InternalRerank:        "prototype",
    InternalRerankProfile: "term_focus",
})
```

These query parameters are intended for internal evaluation and controlled operator experiments, not for broad end-user exposure.

### Retrieval feedback loop groundwork

Persistor also stores explicit, manual retrieval feedback events for operator review. This is groundwork for future tuning, not automatic online learning.

- API only today: `POST /api/v1/admin/retrieval-feedback` to record an event, `GET /api/v1/admin/retrieval-feedback` to inspect a bounded summary.
- Manual and bounded by design: no hidden firehose logging, no silent weighting changes, no autonomous profile switching.
- Outcomes: `helpful`, `unhelpful`, `missed`.
- Derived signals: `confirmed_recall`, `irrelevant_result`, `missed_known_item`, `empty_result`.
- Request metadata can include `search_mode`, inferred-or-supplied `intent`, `internal_rerank`, `internal_rerank_profile`, retrieved/selected/expected node IDs, and an optional note.

Use this for operator comparison runs, QA passes, or product-internal review of search quality.

## Deployment

Persistor ships as a **Go binary + systemd service**. Docker is intentionally not supported —
the binary is small, fast, and runs directly on the host alongside PostgreSQL and Ollama.

### systemd

```ini
# /etc/systemd/system/persistor.service
[Unit]
Description=Persistor
After=postgresql.service ollama.service
Requires=postgresql.service

[Service]
Type=simple
User=persistor
ExecStart=/usr/local/bin/persistor-server
EnvironmentFile=/etc/persistor.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Place environment variables in `/etc/persistor.env` (chmod 600, owned by root).

### Production Keys via Vault

```bash
export ENCRYPTION_PROVIDER=vault
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<your-vault-token>
```

The service fetches the encryption key from Vault at startup, avoiding plaintext keys in environment files.

### Backup / Restore

```bash
./scripts/backup.sh              # pg_dump + verify + encrypt + rotate
./scripts/restore.sh <file>      # Restore from encrypted backup
./scripts/health-check.sh        # Quick health check
```

## License

AGPL-3.0. See [LICENSE](LICENSE) for details. Commercial licensing is available for
organizations that cannot use AGPL — contact [hello@persistor.ai](mailto:hello@persistor.ai).

The Go SDK (`client/`) is Apache-2.0 to allow unrestricted client integration.

See [CHANGELOG.md](CHANGELOG.md) for release history.
