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

All fields are optional. Only provided fields are updated. Returns updated node (200) or **404**.

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

Updates `properties` and/or `weight`. Returns updated edge (200) or **404**.

#### `DELETE /api/v1/edges/:source/:target/:relation` — Delete

```bash
curl -X DELETE http://localhost:3030/api/v1/edges/alice/bob-smith/reports_to \
  -H "Authorization: Bearer $API_KEY"
# {"deleted": true}
```

---

### Search

All search endpoints return `{"nodes": [...], "total": N}`.

#### `GET /api/v1/search` — Full-Text Search

```bash
curl "http://localhost:3030/api/v1/search?q=Smith&type=person&min_salience=0.1&limit=20" \
  -H "Authorization: Bearer $API_KEY"
```

**Query params:**

- `q` (**required**, max 2000 chars) — Search query, matched against node labels.
- `type` — Filter by node type.
- `min_salience` — Minimum salience score (default 0).
- `limit` — Max results (default 20, max 1000).

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

Combines full-text and vector similarity. Falls back to full-text only if embedding generation fails.

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

### Recommended Patterns

1. **Search before creating** — Check if a node exists before creating a duplicate.
2. **Use slug IDs for important entities** — `alice`, `acme-app`, `bob-smith` are easy to reference.
3. **Keep labels search-friendly** — Include context: `"Acme App - data analytics platform"` not just `"Acme App"`.
4. **Use properties for structured data** — Dates, statuses, lists, nested objects all work.
5. **Boost what your user cares about** — When they emphasize something, boost the relevant node.
6. **Supersede, don't delete** — When facts change, create the new node and supersede the old one. This preserves history.
7. **Use context endpoint for rich queries** — One call gets node + neighbors + edges.
8. **Periodic recalc** — Run `salience/recalc` occasionally to keep scores fresh based on access patterns.
