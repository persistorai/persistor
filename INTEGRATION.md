# Persistor — Integration Guide

Complete API reference for the Persistor knowledge graph and vector memory.

**Base URL:** `http://localhost:3030/api/v1`

---

## Quick Start

```bash
# 1. Health check (no auth required)
curl http://localhost:3030/api/v1/health

# 2. Create a node
curl -X POST http://localhost:3030/api/v1/nodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"id": "alice", "type": "person", "label": "Alice Smith", "properties": {"role": "founder"}}'

# 3. Search
curl "http://localhost:3030/api/v1/search?q=Alice" \
  -H "Authorization: Bearer $API_KEY"
```

---

## Authentication

All endpoints except `/health` and `/ready` require a Bearer token:

```
Authorization: Bearer <api-key>
```

API keys are SHA-256 hashed before storage. Each key maps to exactly one tenant. Row-Level Security (RLS) in PostgreSQL ensures complete tenant isolation — you can never see another tenant's data.

---

## Data Model

### Node

| Field            | Type                             | Description                                                                                                    |
| ---------------- | -------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `id`             | string (max 255)                 | Unique identifier. Auto-generated UUID if omitted on create. Slug-style IDs recommended (e.g., `"bob-smith"`). |
| `type`           | string (max 100, **required**)   | Category: `person`, `project`, `concept`, `event`, `place`, etc.                                               |
| `label`          | string (max 10000, **required**) | Human-readable name or description. Used for full-text search.                                                 |
| `properties`     | object (max 64KB JSON)           | Arbitrary metadata. **Encrypted at rest** (AES-256-GCM) — transparent to API consumers.                        |
| `salience_score` | float                            | Auto-calculated importance score. Higher = more important. Affected by access patterns and manual boosts.      |
| `access_count`   | int                              | Times this node has been read.                                                                                 |
| `last_accessed`  | timestamp                        | Last read time.                                                                                                |
| `user_boosted`   | bool                             | Whether a user explicitly boosted this node.                                                                   |
| `superseded_by`  | string or null                   | ID of the node that replaces this one.                                                                         |
| `created_at`     | timestamp                        | Creation time.                                                                                                 |
| `updated_at`     | timestamp                        | Last modification time.                                                                                        |

### Edge

| Field            | Type                           | Description                                                           |
| ---------------- | ------------------------------ | --------------------------------------------------------------------- |
| `source`         | string (max 255, **required**) | Source node ID. Must exist.                                           |
| `target`         | string (max 255, **required**) | Target node ID. Must exist.                                           |
| `relation`       | string (max 255, **required**) | Relationship type: `knows`, `works_on`, `part_of`, `related_to`, etc. |
| `properties`     | object (max 64KB JSON)         | Arbitrary metadata. **Encrypted at rest.**                            |
| `weight`         | float (0–1000, default 1.0)    | Relationship strength.                                                |
| `salience_score` | float                          | Auto-calculated importance.                                           |
| `access_count`   | int                            | Times accessed.                                                       |
| `user_boosted`   | bool                           | Whether manually boosted.                                             |
| `superseded_by`  | string or null                 | Superseding edge reference.                                           |
| `created_at`     | timestamp                      | Creation time.                                                        |
| `updated_at`     | timestamp                      | Last modification time.                                               |

**Composite key:** Edges are uniquely identified by `(source, target, relation)`.

### Episodic Memory Foundations (current internal model)

Phase 3 adds an episodic layer alongside the semantic graph.

- **Semantic memory** remains the public graph surface: nodes, edges, facts, aliases, salience, and search.
- **Episodic memory** is currently an internal foundation used by ingest, belief-aware recall, and the OpenClaw memory plugin.
- There is **not yet a public REST or CLI CRUD surface** for episodes, event records, or recall packs.

#### Episode

An episode groups related event-like memory into a bounded timeline container.

| Field                     | Type                  | Description |
| ------------------------- | --------------------- | ----------- |
| `id`                      | string                | Unique identifier. UUID if omitted in internal create requests. |
| `title`                   | string                | Human-readable episode title. |
| `summary`                 | string                | Optional compact description. |
| `status`                  | string                | `open` or `closed`. |
| `started_at`              | timestamp or null     | Optional lower bound for the episode timeline. |
| `ended_at`                | timestamp or null     | Optional upper bound for the episode timeline. |
| `primary_project_node_id` | string or null        | Optional link back to a project node. |
| `source_artifact_node_id` | string or null        | Optional link back to a source artifact node. |
| `properties`              | object                | Internal metadata such as ingest source and event count. |
| `created_at` / `updated_at` | timestamp           | Audit-friendly lifecycle timestamps. |

#### Event record

An event record stores one event-like memory item linked to an episode.

Supported event kinds: `observation`, `conversation`, `message`, `decision`, `task`, `promise`, `outcome`.

| Field                     | Type                  | Description |
| ------------------------- | --------------------- | ----------- |
| `id`                      | string                | Unique identifier. |
| `episode_id`              | string or null        | Owning episode, when attached. |
| `parent_event_id`         | string or null        | Optional parent event for nesting. |
| `kind`                    | string                | One of the supported event kinds. |
| `title`                   | string                | Short event title. |
| `summary`                 | string                | Optional summary. |
| `occurred_at`             | timestamp or null     | Single event time when known. |
| `occurred_start_at`       | timestamp or null     | Start of a time range. |
| `occurred_end_at`         | timestamp or null     | End of a time range. |
| `confidence`              | float                 | Confidence in the extracted event, `0..1`. Defaults to `1.0`. |
| `evidence`                | array                 | Bounded evidence pointers with `kind`, `ref`, `snippet`, optional `confidence`, timestamps, and properties. |
| `source_artifact_node_id` | string or null        | Optional source artifact link. |
| `properties`              | object                | Internal metadata such as ingest source and entity type/relation. |

#### Event links

Event records can link back to graph nodes with explicit roles, for example `subject`, `decision_maker`, or `decision_topic`.

### Belief tracking (current internal model)

Persistor writes a bounded `_fact_beliefs` summary into node properties when fact evidence exists for a property.

Each property belief includes:

- `preferred_value`
- `preferred_confidence`
- `evidence_count`
- `status`
- `claims[]`

Each claim includes:

- `value`
- `confidence`
- `evidence_count`
- `source_weight`
- `last_observed_at`
- `sources[]`
- `status`
- `preferred`

Current status meanings:

- `supported`: the preferred claim is clearly ahead.
- `contested`: the top competing claim is close enough to the preferred claim to matter operationally.
- `superseded`: evidence exists, but the property no longer has a current stored value.

This is a retrieval and operator-support feature, not yet a standalone belief-management API.

### Recall packs (current internal model)

Recall packs are compact, deterministic summaries assembled for one or more active node IDs.

A recall pack currently contains bounded sections for:

- `core_entities`
- `notable_neighbors`
- `recent_episodes`
- `open_decisions`
- `contradictions`
- `strongest_evidence`

Operator notes:

- Defaults are intentionally small.
- Each section limit is capped at `10` items.
- Recent episodes come from event records linked to the requested node IDs.
- Open decisions are built from event kinds `decision`, `task`, and `promise`, and stay open when the event or its episode still carries an open-like status.
- Contradictions are derived from contested or superseded `_fact_beliefs` entries.

---

## API Reference

### Health

#### `GET /api/v1/health`

Liveness probe. No auth required.

```bash
curl http://localhost:3030/api/v1/health
# {"status": "ok"}
```

#### `GET /api/v1/ready`

Readiness probe — checks database connectivity. No auth required.

```bash
curl http://localhost:3030/api/v1/ready
# {"status": "ok"}        — 200 when healthy
# {"status": "degraded"}  — 503 when DB is down
```

---

### Nodes

#### `POST /api/v1/nodes` — Create

```bash
curl -X POST http://localhost:3030/api/v1/nodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "bob-smith",
    "type": "person",
    "label": "Bob Smith - Team lead",
    "properties": {"role": "team-lead", "notes": "Key team member"}
  }'
```

**Response** (201):

```json
{
  "id": "bob-smith",
  "type": "person",
  "label": "Bob Smith - Team lead",
  "properties": {
    "role": "team-lead",
    "notes": "Key team member"
  },
  "salience_score": 0,
  "access_count": 0,
  "user_boosted": false,
  "created_at": "2026-02-12T00:00:00Z",
  "updated_at": "2026-02-12T00:00:00Z"
}
```

- Omit `id` to auto-generate a UUID.
- Returns **409 Conflict** if the ID already exists.

#### `GET /api/v1/nodes` — List

```bash
# List all nodes (paginated)
curl "http://localhost:3030/api/v1/nodes?limit=50&offset=0" \
  -H "Authorization: Bearer $API_KEY"

# Filter by type
curl "http://localhost:3030/api/v1/nodes?type=person" \
  -H "Authorization: Bearer $API_KEY"

# Filter by minimum salience
curl "http://localhost:3030/api/v1/nodes?min_salience=0.5" \
  -H "Authorization: Bearer $API_KEY"
```

**Query params:** `type`, `min_salience`, `limit` (default 50, max 1000), `offset` (default 0, max 100000)

**Response** (200):

```json
{
  "nodes": [...],
  "has_more": true
}
```

#### `GET /api/v1/nodes/:id` — Get

```bash
curl http://localhost:3030/api/v1/nodes/bob-smith \
  -H "Authorization: Bearer $API_KEY"
```

Returns the node object (200) or **404** if not found.

#### `PUT /api/v1/nodes/:id` — Update

```bash
curl -X PUT http://localhost:3030/api/v1/nodes/bob-smith \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "Bob Smith - Team lead, senior",
    "properties": {"role": "team-lead", "senior": true}
  }'
```

All fields are optional. Only provided fields are updated. **Note:** `properties` replaces the entire object. Use PATCH for partial property updates. Returns updated node (200) or **404**.

#### `PATCH /api/v1/nodes/:id/properties` — Partial Property Update

```bash
curl -X PATCH http://localhost:3030/api/v1/nodes/bob-smith/properties \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "properties": {"senior": true, "department": "engineering"}
  }'
```

Merges provided keys into existing properties. Keys set to `null` are removed. Returns updated node (200) or **404**.

#### `DELETE /api/v1/nodes/:id` — Delete

```bash
curl -X DELETE http://localhost:3030/api/v1/nodes/bob-smith \
  -H "Authorization: Bearer $API_KEY"
# {"deleted": true}
```

**Cascades:** Deleting a node also deletes all connected edges.

---

### Edges

#### `POST /api/v1/edges` — Create

```bash
curl -X POST http://localhost:3030/api/v1/edges \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "alice",
    "target": "bob-smith",
    "relation": "reports_to",
    "properties": {"since": "2024"},
    "weight": 1.0
  }'
```

- Both `source` and `target` nodes must exist (returns 400 otherwise).
- Returns **409 Conflict** if edge with same `(source, target, relation)` exists.
- `weight` is optional (default behavior determined by DB).

#### `GET /api/v1/edges` — List

```bash
# All edges
curl "http://localhost:3030/api/v1/edges" -H "Authorization: Bearer $API_KEY"

# Filter by source, target, or relation
curl "http://localhost:3030/api/v1/edges?source=alice" -H "Authorization: Bearer $API_KEY"
curl "http://localhost:3030/api/v1/edges?relation=works_on" -H "Authorization: Bearer $API_KEY"
```

**Query params:** `source`, `target`, `relation`, `limit` (default 50, max 1000), `offset` (default 0)

**Response** (200):

```json
{
  "edges": [...],
  "has_more": true
}
```

#### `PUT /api/v1/edges/:source/:target/:relation` — Update

```bash
curl -X PUT http://localhost:3030/api/v1/edges/alice/bob-smith/reports_to \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"properties": {"since": "2024", "quality": "close"}, "weight": 2.0}'
```

Updates `properties` and/or `weight`. **Note:** `properties` replaces the entire object. Use PATCH for partial property updates. Returns updated edge (200) or **404**.

#### `PATCH /api/v1/edges/:source/:target/:relation/properties` — Partial Property Update

```bash
curl -X PATCH http://localhost:3030/api/v1/edges/alice/bob-smith/reports_to/properties \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"properties": {"quality": "close"}}'
```

Merges provided keys into existing properties. Keys set to `null` are removed. Returns updated edge (200) or **404**.

#### `DELETE /api/v1/edges/:source/:target/:relation` — Delete

```bash
curl -X DELETE http://localhost:3030/api/v1/edges/alice/bob-smith/reports_to \
  -H "Authorization: Bearer $API_KEY"
# {"deleted": true}
```

---

### Search

All search endpoints return `{"nodes": [...], "total": N}`.

### Alias Support and Current API Surface

Persistor stores normalized aliases for nodes and uses them in retrieval and duplicate analysis.

- Exact lookup can resolve by canonical label or stored alias.
- Full-text and hybrid search consider alias matches alongside labels.
- Duplicate candidate detection compares normalized labels and normalized aliases.
- Alias normalization is lowercase + trimmed + collapsed internal whitespace, so equivalent spellings match reliably.
- There is currently **no public REST or CLI alias CRUD surface**. Alias persistence exists in the underlying model and services, but operators should treat alias creation as an internal/system workflow for now.

#### `GET /api/v1/search` — Full-Text Search

```bash
curl "http://localhost:3030/api/v1/search?q=Smith&type=person&min_salience=0.1&limit=20" \
  -H "Authorization: Bearer $API_KEY"
```

**Query params:**

- `q` (**required**, max 2000 chars) — Search query, matched against node labels and stored aliases.
- `type` — Filter by node type.
- `min_salience` — Minimum salience score (default 0).
- `limit` — Max results (default 20, max 1000).

Alias matches are ranked strongly for exact and normalized matches, then blended with normal text ranking.

#### `GET /api/v1/search/semantic` — Vector Similarity Search

```bash
curl "http://localhost:3030/api/v1/search/semantic?q=team+relationships&limit=10" \
  -H "Authorization: Bearer $API_KEY"
```

Generates an embedding from `q` via Ollama and finds nodes with similar embeddings. Results include a `score` field. Returns **502** if embedding service is unavailable.

**Query params:** `q` (**required**), `limit` (default 10).

#### `GET /api/v1/search/hybrid` — Combined Text + Vector Search

```bash
curl "http://localhost:3030/api/v1/search/hybrid?q=active+projects&limit=10" \
  -H "Authorization: Bearer $API_KEY"
```

Combines full-text and vector similarity. Falls back to full-text only if embedding generation fails. The text side is alias-aware, so a strong alias match can still surface even when the canonical label differs.

**Query params:** `q` (**required**), `limit` (default 10).

---

### Graph Traversal

#### `GET /api/v1/graph/neighbors/:id` — Direct Neighbors

```bash
curl "http://localhost:3030/api/v1/graph/neighbors/alice?limit=100" \
  -H "Authorization: Bearer $API_KEY"
```

Returns nodes directly connected to the given node (1 hop). **Query params:** `limit` (default 100).

#### `GET /api/v1/graph/traverse/:id` — BFS Traversal

```bash
curl "http://localhost:3030/api/v1/graph/traverse/alice?hops=3" \
  -H "Authorization: Bearer $API_KEY"
```

Breadth-first traversal up to N hops from the starting node. **Query params:** `hops` (default 2, max 10).

#### `GET /api/v1/graph/context/:id` — Full Context

```bash
curl "http://localhost:3030/api/v1/graph/context/alice" \
  -H "Authorization: Bearer $API_KEY"
```

Returns the node, its neighbors, and connecting edges in one call. Ideal for "tell me everything about X."

#### `GET /api/v1/graph/path/:from/:to` — Shortest Path

```bash
curl "http://localhost:3030/api/v1/graph/path/alice/acme-app" \
  -H "Authorization: Bearer $API_KEY"
```

Returns `{"path": [...]}` with ordered node IDs, or **404** if no path exists.

---

### Bulk Operations

Both endpoints accept a raw JSON array (max 1000 items). They perform upserts — existing items are updated, new items are created.

#### `POST /api/v1/bulk/nodes`

```bash
curl -X POST http://localhost:3030/api/v1/bulk/nodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '[
    {"id": "acme-app", "type": "project", "label": "Acme App - data analytics platform"},
    {"id": "techcorp", "type": "company", "label": "TechCorp - cloud services company"}
  ]'
# {"upserted": 2}
```

#### `POST /api/v1/bulk/edges`

```bash
curl -X POST http://localhost:3030/api/v1/bulk/edges \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '[
    {"source": "alice", "target": "acme-app", "relation": "created"},
    {"source": "alice", "target": "techcorp", "relation": "works_at"}
  ]'
# {"upserted": 2}
```

---

### Salience Management

#### `POST /api/v1/salience/boost/:id` — Boost Node

```bash
curl -X POST http://localhost:3030/api/v1/salience/boost/bob-smith \
  -H "Authorization: Bearer $API_KEY"
```

Manually boosts a node's salience score and sets `user_boosted: true`. Returns the updated node.

#### `POST /api/v1/salience/supersede` — Mark Node Superseded

```bash
curl -X POST http://localhost:3030/api/v1/salience/supersede \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"old_id": "outdated-fact", "new_id": "corrected-fact"}'
```

Marks `old_id` as superseded by `new_id`. The old node's `superseded_by` field is set. Both IDs are required and must differ.

#### `POST /api/v1/salience/recalc` — Recalculate All Salience

```bash
curl -X POST http://localhost:3030/api/v1/salience/recalc \
  -H "Authorization: Bearer $API_KEY"
# {"updated": 150}
```

Triggers a full recalculation of salience scores for all nodes in the tenant. Only one recalculation runs at a time per tenant (returns **409 Conflict** if already running).

---

### Admin Maintenance and Duplicate Review

#### `POST /api/v1/admin/reprocess-nodes` — Rebuild Derived Node Data

```bash
curl -X POST http://localhost:3030/api/v1/admin/reprocess-nodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"batch_size":100,"search_text":true,"embeddings":true}'
```

Use this for **targeted backfill** of existing nodes.

- `search_text: true` rebuilds stored `search_text` for scanned nodes.
- `embeddings: true` queues embeddings for scanned nodes that are still missing them.
- Results include `scanned`, `updated_search`, `queued_embeddings`, and remaining counts.

Use `reprocess-nodes` when you mainly need to repair or backfill missing derived data. It does not scan stale fact evidence or duplicate candidates.

#### `POST /api/v1/admin/maintenance/run` — Explicit Maintenance Pass

```bash
curl -X POST http://localhost:3030/api/v1/admin/maintenance/run \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "batch_size": 100,
    "refresh_search_text": true,
    "refresh_embeddings": true,
    "scan_stale_facts": true,
    "include_duplicate_candidates": true
  }'
```

This is the broader operator workflow for an explicit maintenance pass over nodes that need attention.

- `refresh_search_text` updates stored search text when the rebuilt text differs.
- `refresh_embeddings` queues embeddings for scanned nodes that still lack them.
- `scan_stale_facts` counts nodes carrying `_fact_evidence` and nodes with superseded or conflicting fact history.
- `include_duplicate_candidates` counts likely duplicate pairs for the tenant in the current pass.

The response includes:

- `scanned`
- `updated_search_text`
- `queued_embeddings`
- `stale_fact_nodes`
- `superseded_nodes`
- `duplicate_candidate_pairs`
- `remaining_search_text`
- `remaining_embeddings`
- `remaining_maintenance_nodes`

Use `maintenance/run` when you want an operator health sweep, not just a backfill.

#### `GET /api/v1/admin/merge-suggestions` — Inspect Explainable Duplicate Candidates

```bash
curl "http://localhost:3030/api/v1/admin/merge-suggestions?type=person&limit=10&min_score=0.7" \
  -H "Authorization: Bearer $API_KEY"
```

Returns scored duplicate suggestions without mutating data.

Each suggestion contains:

- `canonical` — the higher-salience node chosen as the default keep target
- `duplicate` — the lower-salience node in the pair
- `score` — rounded `0.00` to `1.00`
- `reasons` — explainable signals such as:
  - `same_normalized_label`
  - `label_alias_overlap`
  - `shared_names`
  - `matching_identity_properties`

Current duplicate suggestions are intentionally conservative:

- only active, non-superseded nodes are compared
- only pairs of the same node type are compared
- suggestions are based on shared normalized labels and aliases, plus a few identity-like property matches
- no automatic merge is performed

#### Ingest Entity Resolution Behavior

During ingest, Persistor resolves entities in layers:

1. exact node ID match
2. exact label or alias-aware exact lookup
3. search-backed candidates scored by normalized label similarity, substring overlap, and optional type match

Practical confidence model:

- below `0.50`: ignored
- `0.50` to `<0.93`: candidate can be considered, but may remain ambiguous
- `>= 0.93`: eligible for auto-match
- if the best candidate is too close to the next one (gap under `0.08`, or under `0.04` for very high-confidence ties), ingest treats the result as ambiguous and avoids a silent merge

This means Persistor prefers false negatives over unsafe false-positive merges.

#### `POST /api/v1/admin/retrieval-feedback` — Record Explicit Retrieval Feedback

```bash
curl -X POST http://localhost:3030/api/v1/admin/retrieval-feedback \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Persistor deploy fix",
    "search_mode": "hybrid",
    "internal_rerank": "prototype",
    "internal_rerank_profile": "term_focus",
    "outcome": "helpful",
    "retrieved_node_ids": ["deploy-runbook", "incident-123"],
    "selected_node_ids": ["deploy-runbook"],
    "note": "Operator confirmed the top result was the right runbook"
  }'
```

Creates one manual, operator-visible feedback event. This surface is intentionally bounded and explicit. Persistor does **not** automatically log every search, retrain models, or change retrieval weights from these events.

Request fields:

- `query` (required, 1 to 500 chars)
- `outcome` (required): `helpful`, `unhelpful`, or `missed`
- optional: `search_mode`, `intent`, `internal_rerank`, `internal_rerank_profile`
- optional node ID lists: `retrieved_node_ids`, `selected_node_ids`, `expected_node_ids` (max 20 each)
- optional `note` (max 500 chars)

Derived signals are computed server-side from the request:

- `confirmed_recall`
- `irrelevant_result`
- `missed_known_item`
- `empty_result`

If `intent` is omitted, Persistor derives it from the query before storing the event.

#### `GET /api/v1/admin/retrieval-feedback` — Retrieval Feedback Summary

```bash
curl "http://localhost:3030/api/v1/admin/retrieval-feedback?limit=10" \
  -H "Authorization: Bearer $API_KEY"
```

Returns a bounded summary over the most recent events, with:

- `total_events`
- `outcome_counts`
- `signal_counts`
- `recent_events`
- `query_breakdown` grouped by normalized query + search mode

`limit` defaults to `25` and is capped at `100`.

#### Hybrid Search Prototype Reranking

The standard hybrid endpoint remains the default retrieval path. Phase 4 adds an **internal, opt-in** bounded reranking pass for comparison testing and operator experiments.

Enable it by adding query params to `GET /api/v1/search/hybrid`:

- `internal_rerank=prototype`
- optional `internal_rerank_profile=default|term_focus|salience_focus`

Example:

```bash
curl "http://localhost:3030/api/v1/search/hybrid?q=Persistor%20deploy%20fix&limit=5&internal_rerank=prototype&internal_rerank_profile=term_focus" \
  -H "Authorization: Bearer $API_KEY"
```

Behavior notes:

- without `internal_rerank=prototype`, hybrid search behaves as before
- reranking only reorders the retrieved hybrid candidate set, it does not widen public result limits
- candidate overfetch is bounded to `limit * 3`, capped at `50`, then trimmed back to the requested `limit`
- unknown or empty profile names normalize to the `default` weighting profile

Named profiles:

- `default` — balanced term, salience, and original-rank weighting
- `term_focus` — more aggressive term and query-coverage weighting, less salience bias
- `salience_focus` — stronger salience and original-rank influence, lighter term weighting

#### Eval Fixture Comparison Flow

The CLI evaluation harness can compare named prototype rerank profiles without changing production defaults.

```bash
persistor eval run \
  --fixture ./testdata/eval/scout-memory-phase2.json \
  --compare-rerank-profile term_focus \
  --compare-rerank-profile salience_focus
```

How it works:

- baseline run: the fixture executes once with the prototype reranker using the `default` profile
- comparison runs: the same fixture is cloned and every `search_mode: "hybrid_rerank"` question is re-run once per named profile
- output: a JSON comparison report containing the baseline plus one report per requested profile

For a per-question table, run a normal fixture execution with `--format table`. The comparison mode does not currently emit the table formatter.

#### Operator Guidance: Which Workflow to Use?

- Use **`reprocess-nodes`** for missing or outdated derived data on existing nodes, especially after improvements to search text or embedding generation.
- Use **`maintenance/run`** for an operator sweep that also surfaces stale fact evidence and duplicate-candidate volume.
- Use **`merge-suggestions`** for manual review before any human-driven merge or supersession workflow.
- Use **prototype reranking** only for internal search-quality experiments, fixture comparisons, and controlled operator analysis. It is optional and off by default.
- Use **retrieval feedback** to capture explicit review events and summarize failure patterns. It is groundwork for future tuning, not an automatic adaptation loop today.
- A future **full re-ingest** is the right tool only when source extraction logic, chunking, or source material itself changed enough that existing stored nodes are no longer a trustworthy representation. Do not use full re-ingest for routine search-text or embedding refresh.

---

### WebSocket — Real-Time Notifications

```
ws://localhost:3030/api/v1/ws
```

Requires the same Bearer token auth. Connect to receive real-time notifications when nodes or edges are created, updated, or deleted (driven by PostgreSQL LISTEN/NOTIFY). Messages are tenant-scoped — you only receive events for your own data.

---

## Encryption

Node and edge `properties` are AES-256-GCM encrypted at rest in PostgreSQL. This is **transparent to API consumers** — you send and receive plain JSON. The encryption key is configured via:

- **Static:** `ENCRYPTION_PROVIDER=static` + `ENCRYPTION_KEY` (64 hex chars = 32 bytes)
- **Vault:** `ENCRYPTION_PROVIDER=vault` + `VAULT_ADDR` + `VAULT_TOKEN` (key fetched from HashiCorp Vault)

---

## Tenant Isolation

PostgreSQL Row-Level Security ensures complete data isolation between tenants. One API key = one tenant. There is no way to access another tenant's nodes, edges, or search results.

---

## Error Responses

All errors follow a consistent format:

```json
{
  "error": {
    "code": "validation_error",
    "message": "type is required"
  }
}
```

Common error codes: `invalid_request`, `validation_error`, `not_found`, `conflict`, `internal_error`.

---

## Rate Limits

- **100 requests/second** per IP with a burst allowance of 200.
- **Max body size:** 10 MB.
- **Max bulk items:** 1000 per request.
- **Max search query:** 2000 characters.
- **Max traversal hops:** 10.

---

## Agent Integration

This section covers integration for AI agents using Persistor as their memory backend.

### Connection Details

- **URL:** `http://localhost:3030`
- **API Key:** Stored in Vault at `secret/your-app/memory-service`

```bash
# Retrieve API key
source $VAULT_ENV_FILE
API_KEY=$(vault kv get -field=api_key secret/your-app/memory-service)
```

### How to Model the Knowledge Graph

Use **nodes** for entities and **edges** for relationships:

| Entity Type | `type` value | Example IDs                              |
| ----------- | ------------ | ---------------------------------------- |
| People      | `person`     | `alice`, `bob-smith`                     |
| Projects    | `project`    | `acme-app`, `persistor`                  |
| Companies   | `company`    | `techcorp`, `my-company`                 |
| Concepts    | `concept`    | `repository-pattern`, `related-research` |
| Events      | `event`      | `product-launch`                         |
| Places      | `place`      | `san-francisco`                          |
| Animals     | `animal`     | `moose-142`                              |

**ID conventions:** Use lowercase slug-style IDs (`bob-smith`, `acme-app`) for human-readable entities. Let UUIDs auto-generate for transient or programmatic entries.

**Labels:** Write descriptive labels — they're what full-text search matches against. `"Bob Smith - Team lead, senior engineer"` is better than `"Bob Smith"`.

**Properties:** Use for rich metadata that doesn't fit in the label:

```json
{
  "id": "acme-app",
  "type": "project",
  "label": "Acme App - data analytics platform for real-time insights",
  "properties": {
    "tech_stack": ["Go", "Python", "PostgreSQL", "Redis"],
    "status": "active",
    "repo": "https://github.com/example/acme-app",
    "domain": "acme-app.example.com"
  }
}
```

### Common Edge Relations

| Relation     | Meaning                    | Example                     |
| ------------ | -------------------------- | --------------------------- |
| `knows`      | Person knows person        | alice → bob-smith           |
| `created`    | Person created project     | alice → acme-app            |
| `works_at`   | Person works at company    | alice → techcorp            |
| `works_on`   | Person works on project    | alice → acme-app            |
| `part_of`    | Entity belongs to group    | acme-app → my-company       |
| `related_to` | General association        | related-research → acme-app |
| `located_in` | Entity is in a place       | alice → san-francisco       |
| `depends_on` | Project depends on project | acme-app → persistor        |

### Daily Operations

#### Before answering a question about a topic, search first

```bash
# Text search — fast, matches labels
curl "http://localhost:3030/api/v1/search?q=Acme+App" -H "Authorization: Bearer $API_KEY"

# Semantic search — finds conceptually related nodes
curl "http://localhost:3030/api/v1/search/semantic?q=data+analytics+project" -H "Authorization: Bearer $API_KEY"

# Hybrid — best of both
curl "http://localhost:3030/api/v1/search/hybrid?q=active+projects" -H "Authorization: Bearer $API_KEY"
```

#### "Tell me everything about X" — Use context

```bash
curl "http://localhost:3030/api/v1/graph/context/alice" -H "Authorization: Bearer $API_KEY"
```

This returns the node, all neighbors, and connecting edges in one call.

#### For deeper exploration — Use traverse

```bash
curl "http://localhost:3030/api/v1/graph/traverse/alice?hops=2" -H "Authorization: Bearer $API_KEY"
```

#### When your user says something is important — Boost it

```bash
curl -X POST "http://localhost:3030/api/v1/salience/boost/bob-smith" -H "Authorization: Bearer $API_KEY"
```

#### When a fact changes — Supersede it

```bash
curl -X POST "http://localhost:3030/api/v1/salience/supersede" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"old_id": "old-fact-id", "new_id": "new-fact-id"}'
```

#### Batch operations — Use bulk endpoints for efficiency

When you need to create many nodes/edges at once (e.g., ingesting a new topic area), use bulk endpoints instead of individual calls:

```bash
curl -X POST http://localhost:3030/api/v1/bulk/nodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '[
    {"id": "node-1", "type": "concept", "label": "First concept"},
    {"id": "node-2", "type": "concept", "label": "Second concept"}
  ]'
```

### OpenClaw memory-persistor extension

The OpenClaw `memory-persistor` extension combines file memory and Persistor graph retrieval.

#### Registered tools and commands

- Tools: `memory_search`, `memory_get`
- CLI: `memory-kg status`, `memory-kg search <query>`

#### Session-aware retrieval context

`memory_search` accepts the normal file-memory params plus optional session context fields:

- `currentSessionEntities` (aliases: `sessionEntities`, `entities`)
- `recentMessages` (aliases: `messages`, `recentTurns`)
- `activeWorkContext` (aliases: `workContext`, `activeTask`, `taskContext`)

Current behavior:

- values are normalized, deduplicated, and truncated to small bounded lists
- the plugin builds up to 4 query variants from the original query plus session context
- source preference is inferred heuristically as `file`, `persistor`, or `both`
- context lightly boosts matching file snippets and Persistor nodes before final merge
- response metadata reports `persistorAvailable`, source counts, `sourcePreference`, `queryVariants`, and `currentSessionEntities`

Example tool payload:

```json
{
  "query": "who is working on persistor",
  "currentSessionEntities": ["Brian"],
  "recentMessages": ["Brian is implementing session-aware retrieval"],
  "activeWorkContext": ["persistor repo retrieval task"],
  "maxResults": 8,
  "minScore": 0.2
}
```

`memory_get` behavior:

- file-like paths such as `./foo.md`, `/tmp/foo.md`, and `memory/tasks.md` go to file memory directly
- UUIDs and other strings first try Persistor node lookup
- when configured, Persistor lookups include graph context (neighbors and edges)
- if Persistor lookup fails, the call falls back to file memory

#### Extension config

Current config keys:

```json
{
  "persistor": {
    "url": "http://localhost:3030",
    "apiKey": "${PERSISTOR_API_KEY}",
    "timeout": 3000,
    "searchMode": "hybrid",
    "searchLimit": 10
  },
  "weights": {
    "file": 1.0,
    "persistor": 0.9
  },
  "persistorContextOnGet": true
}
```

### Recommended Patterns

1. **Search before creating** — Check if a node exists before creating a duplicate. Alias-aware lookup makes this more effective than label-only matching.
2. **Use slug IDs for important entities** — `alice`, `acme-app`, `bob-smith` are easy to reference.
3. **Keep labels search-friendly** — Include context: `"Acme App - data analytics platform"` not just `"Acme App"`.
4. **Use properties for structured data** — Dates, statuses, lists, nested objects all work.
5. **Boost what your user cares about** — When they emphasize something, boost the relevant node.
6. **Supersede, don't delete** — When facts change, create the new node and supersede the old one. This preserves history.
7. **Use context endpoint for rich queries** — One call gets node + neighbors + edges.
8. **Use `reprocess-nodes` for targeted backfills** — best for missing `search_text` or embeddings on existing nodes.
9. **Use `maintenance/run` for operator sweeps** — best for refresh work, stale fact evidence counts, and duplicate-candidate visibility.
10. **Treat merge suggestions as review input** — they explain likely duplicates, but they do not merge automatically.
11. **Periodic recalc** — Run `salience/recalc` occasionally to keep scores fresh based on access patterns.
12. **Treat episodic ingest as conservative** — it only creates episode/event records for strong event signals, not for every extracted entity or relationship.
13. **Use recall packs as bounded context** — they are designed to brief an agent on active topics, not to dump full history.
14. **Interpret `_fact_beliefs` as internal summaries** — useful for retrieval and operator visibility, but not a public write contract.
15. **Pass session context into the OpenClaw plugin when available** — `currentSessionEntities`, `recentMessages`, and `activeWorkContext` improve ranking without changing the core query surface.
