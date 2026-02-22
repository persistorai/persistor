# CLAUDE.md — Persistor

## Project

Persistor is a knowledge graph service with PostgreSQL + pgvector backend.
Go 1.25+, Gin HTTP framework, goose migrations, multi-tenant with RLS.

Repo: `github.com/persistorai/persistor`
Local: `/home/brian/code/persistor`

## Build Gate

Before committing, ALL of these must pass:

```bash
cd /home/brian/code/persistor
go build ./...
go vet ./...
~/go/bin/golangci-lint run ./...
go test ./...
```

If any fail, fix them. Do not commit broken code. Do not skip tests.

## Go Standards

Follow the project standards at `/home/brian/code/standards/go/`.

## Type Safety

- ZERO use of `interface{}` or `any` in function signatures — use concrete types or named interfaces
- Exception: `map[string]any` for JSON properties is acceptable (it's the domain model)
- ZERO type assertions without comma-ok pattern: always `v, ok := x.(Type)`
- Use typed errors: define sentinel errors or custom error types, not raw `fmt.Errorf` for control flow
- Return concrete types from constructors, accept interfaces in functions

## File Structure

- No file over 300 lines. If approaching 250, plan how to split.
- One concern per file. Two responsibilities = two files.
- Internal packages follow Go convention: `internal/` is not importable externally
- Models in `internal/models/`, stores in `internal/store/`, services in `internal/service/`
- API handlers in `internal/api/`

## Functions

- Max 50 lines per function. Extract helpers.
- No more than 3 levels of nesting. Use early returns.
- `context.Context` is always the first parameter
- Errors are always the last return value
- Name return values only when it improves readability (named returns for documentation, not naked returns)

## Error Handling

- Every error must be handled. No `_ = someFunc()` that returns an error.
- Wrap errors with context: `fmt.Errorf("creating node: %w", err)`
- Use `errors.Is()` and `errors.As()` for error checking, not string matching
- Sentinel errors in `internal/models/errors.go`
- User-facing errors: clear and actionable, not stack traces
- Log at the boundary (handler), not deep in service/store layers

## Interfaces

- Define interfaces where they're CONSUMED, not where they're implemented
- Keep interfaces small — 1-3 methods preferred
- Domain interfaces live in `internal/domain/interfaces.go`
- Store interfaces live near store implementations
- Use dependency injection via constructor functions: `NewNodeService(store NodeStore, ...)`

## Naming

- Interfaces: PascalCase, no `I` prefix (`NodeStore`, not `INodeStore`)
- Constructors: `New<Type>` (`NewNodeService`)
- Functions: camelCase internally, PascalCase exported
- Constants: PascalCase for exported, camelCase for unexported
- Files: snake_case (`node_read.go`, `graph_traverse.go`)
- Packages: short, lowercase, no underscores

## Imports

- Order: stdlib → external → internal (goimports handles this)
- No dot imports
- No blank imports except for driver registration (`_ "github.com/lib/pq"`)

## Database

- PostgreSQL 18+ with pgvector
- goose for migrations in `internal/db/migrations/`
- Row-level security (RLS) via `app.tenant_id` session variable
- No foreign keys (by design — referential integrity in app layer)
- Always use parameterized queries (`$1`, `$2`), never string interpolation
- Transactions for multi-statement operations
- Connection pooling via `internal/dbpool/`

## Testing

- Every package has `*_test.go` files
- Use `testify` assertions where already established
- Mock interfaces, not implementations — use `mocks_test.go` per package
- Table-driven tests for multiple cases
- Test edge cases: missing nodes, duplicate edges, invalid UUIDs, tenant isolation
- Integration tests use test database with fresh schema per run

## Security

- AES-256-GCM encryption for sensitive fields (`internal/crypto/`)
- Multi-tenant isolation via PostgreSQL RLS — every query scoped to tenant
- API key auth via middleware (`internal/middleware/`)
- No raw SQL from user input — always parameterized
- Validate all input at the handler/model layer before it reaches the store

## Commits

- Conventional: `feat:`, `fix:`, `test:`, `chore:`, `docs:`
- One logical change per commit
- Message explains WHY, not just WHAT

## Architecture

```
cmd/persistor/         # Main entry point
client/                # Go client SDK
internal/
  api/                 # HTTP handlers (Gin)
  config/              # Configuration loading
  crypto/              # AES-256-GCM encryption
  db/migrations/       # goose SQL migrations
  dbpool/              # Connection pool
  domain/              # Service interfaces
  graphql/             # GraphQL schema + resolvers
  httputil/            # HTTP helpers
  metrics/             # Prometheus metrics
  middleware/          # Auth, tenant, logging
  models/              # Domain types + validation
  security/            # Rate limiting, etc.
  service/             # Business logic
  store/               # PostgreSQL data access
  ws/                  # WebSocket support
extensions/            # OpenClaw plugin extensions
scripts/               # Migration scripts
```

## Key Interfaces (in internal/domain/interfaces.go)

- `NodeService` — CRUD + migrate nodes
- `EdgeService` — CRUD edges
- `SearchService` — full-text, semantic, hybrid search
- `GraphService` — neighbors, traverse, context, shortest path
- `SalienceService` — boost, supersede, recalculate
- `BulkService` — bulk upsert nodes/edges
- `AuditService` — query + purge audit log
- `HistoryService` — property change tracking

## Refactoring Rule

If you move, rename, or change the signature of any function:
- Update EVERY file that imports or references it
- Update EVERY test that calls or mocks it
- ALL tests must still pass after your changes
- Do NOT leave broken imports or stale mocks
