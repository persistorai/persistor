# AGENTS.md — Persistor

## Project

Persistor is a memory system for AI agents: the durable `.md` notes an agent
writes are the source of truth, and a PostgreSQL full-text index over them is
rebuildable. No entity graph, no vectors/embeddings — full-text retrieval over
prose. Go 1.25+, goose migrations, multi-tenant with RLS.

Repo: `github.com/briancolinger/persistor`

## Build Gate

Before committing, ALL of these must pass:

```bash
go build ./...
go vet ./...
~/go/bin/golangci-lint run ./...
go test ./...
```

If any fail, fix them. Do not commit broken code. Do not skip tests.

## Go Standards

Follow standard Go best practices (Effective Go, Google Go style guide).

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
- The memory engine lives in `internal/index/` (indexer, chunking, FTS
  search, store); the retrieval eval in `internal/eval/`; migrations + pool +
  version in `internal/db/`, `internal/dbpool/`, `internal/config/`

## Functions

- Max 50 lines per function. Extract helpers.
- No more than 3 levels of nesting. Use early returns.
- `context.Context` is always the first parameter
- Errors are always the last return value
- Name return values only when it improves readability (named returns for documentation, not naked returns)

## Error Handling

- Every error must be handled. No `_ = someFunc()` that returns an error.
- Wrap errors with context: `fmt.Errorf("indexing note: %w", err)`
- Use `errors.Is()` and `errors.As()` for error checking, not string matching
- Typed errors live with the code that returns them (e.g. `index.SelfSupersedeError`)
- User-facing errors: clear and actionable, not stack traces
- Log at the boundary (handler), not deep in service/store layers

## Interfaces

- Define interfaces where they're CONSUMED, not where they're implemented
- Keep interfaces small — 1-3 methods preferred (e.g. `eval.SearchClient` is one method)
- Define interfaces where they're consumed; the concrete `index.Store` is passed in
- Use dependency injection via constructor functions: `index.NewStore(pool, log)`, `eval.NewRunner(search)`

## Naming

- Interfaces: PascalCase, no `I` prefix (`SearchClient`, not `ISearchClient`)
- Constructors: `New<Type>` (`NewStore`, `NewIndexer`)
- Functions: camelCase internally, PascalCase exported
- Constants: PascalCase for exported, camelCase for unexported
- Files: snake_case (`notes_read.go`, `writeid.go`)
- Packages: short, lowercase, no underscores

## Imports

- Order: stdlib → external → internal (goimports handles this)
- No dot imports
- No blank imports except for driver registration (`_ "github.com/lib/pq"`)

## Database

- PostgreSQL 18+ (no pgvector — FTS only)
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
- Test edge cases: missing notes, supersession, self-supersede, invalid UUIDs, tenant isolation
- Integration tests use test database with fresh schema per run

## Security

- Multi-tenant isolation via PostgreSQL RLS — every query scoped to `app.tenant_id`
- The local index is plaintext (FTS needs it); at-rest encryption returns with the
  hosted/public-MCP endpoint, not before
- No raw SQL from user input — always parameterized
- Personal data lives only in a local disposable DB and the private notes repo;
  this product repo's test fixtures stay synthetic

## Commits

- Conventional: `feat:`, `fix:`, `test:`, `chore:`, `docs:`
- One logical change per commit
- Message explains WHY, not just WHAT

## Architecture

```text
cmd/persistor-cli/     # the `persistor` CLI: reindex, brief, search, consolidate, eval
cmd/persistor-mcp/     # the MCP server (stdio): memory_search, memory_get, memory_write, brief
internal/
  index/               # memory engine: file-sync indexer, chunking, FTS search
  eval/                # deterministic retrieval eval (recall@k) + baselines
  db/                  # goose migrations (one schema) + runner
  db/migrations/       # the SQL migration
  dbpool/              # Connection pool
  config/              # build-time Version only (env is read at point of use)
scripts/               # loop-gate.sh, git hooks
```

## Refactoring Rule

If you move, rename, or change the signature of any function:

- Update EVERY file that imports or references it
- Update EVERY test that calls or mocks it
- ALL tests must still pass after your changes
- Do NOT leave broken imports or stale mocks

## Semantic Refactoring (AST-aware tools)

**For renames and cross-codebase refactoring, use semantic tools instead of grep:**

```bash
# Go: Rename a symbol across the entire project (AST-aware, understands types/interfaces)
~/go/bin/gopls rename -w path/to/file.go:LINE:COL "NewName"

# Go: Find all references to a symbol
~/go/bin/gopls references path/to/file.go:LINE:COL

# Go: Type-check the project
/usr/local/go/bin/go vet ./...
```

**Why not grep?** grep is text matching, not code understanding. It misses
interface implementations, embedded struct promotions, and re-exports, and
produces false positives from comments. gopls understands the full Go type
system.
