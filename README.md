# Persistor

Memory for AI agents. The durable markdown notes an agent writes are the source
of truth; a PostgreSQL full-text index over them makes them searchable. Index
everything; decide relevance at query time. No entity extraction, no graph, no
vectors — notes in, ranked notes out.

## How it works

An agent already writes correct prose it authored and reads natively. Persistor
treats those `.md` files as canonical and keeps a rebuildable full-text index
beside them: a content-hash file-sync indexer re-indexes only changed files,
parses frontmatter, chunks on headings, and serves ranked retrieval over Postgres
`tsvector`. If the index is ever lost, re-running the indexer rebuilds it from the
notes.

## What's here

- `internal/index` — the engine: the file-sync indexer, frontmatter parsing,
  heading-aware chunking, FTS retrieval (`SearchNotes`), the bounded working-set,
  and the consolidation write path.
- `internal/eval` — a deterministic retrieval eval (recall@k) that gates every
  change against baselines.
- `internal/db`, `internal/dbpool`, `internal/config` — the migration + runner,
  the connection pool, and the build-time version.
- `cmd/persistor-cli` — the `persistor` CLI (five commands).
- `cmd/persistor-mcp` — the MCP server (stdio).
- `harness/claude-code` — install artifacts for wiring Persistor into Claude Code.

## CLI

- `persistor reindex` — walk the watched note roots (a notes repo + optionally
  Claude Code's auto-memory), apply migrations, and re-index changed files.
- `persistor brief` — assemble the working-set: every pinned Core note plus the
  Tail notes most relevant to the current project, under a token budget. Degrades
  to Core-read-from-disk if the index is down.
- `persistor search <query>` — run the FTS retrieval and print ranked notes.
  Excludes superseded notes by default; `--include-superseded` shows history.
- `persistor consolidate --plan <plan.json>` — apply a consolidation plan: write
  prose notes (with supersession) and reindex. The plan's JSON shape is documented
  in `persistor consolidate --help`. Writes land under `--write-dir` (default
  `<notes-dir>/memory/atomic`).
- `persistor eval --fixture <f>` — score the index against the baselines
  (full retrieval vs static `MEMORY.md` vs no memory).

## MCP server

`cmd/persistor-mcp` serves four tools over stdio for clients such as Claude Code:
`memory_search`, `memory_get`, `memory_write`, `brief`.

## Quick start

```bash
createdb persistor
export DATABASE_URL=postgres://persistor:...@localhost:5432/persistor?sslmode=disable
export PERSISTOR_TENANT_ID=<uuid>
export PERSISTOR_NOTES_DIR=/path/to/notes-repo

go build ./...
persistor reindex                    # applies migrations + indexes the notes
persistor eval --fixture <f> --format table
```

### Environment

- `DATABASE_URL` — Postgres connection string (required).
- `PERSISTOR_TENANT_ID` — tenant UUID; namespaces every row (required).
- `PERSISTOR_NOTES_DIR` — the notes repo root, the primary watched root (required).
- `PERSISTOR_WRITE_DIR` — where `consolidate`/`memory_write` write new notes
  (optional; default `<notes-dir>/memory/atomic`).
- `CLAUDE_MEMORY_DIR` — Claude Code's auto-memory dir to also index (optional).

## Schema

One table per concern, all RLS-tenant-isolated:

- `sources(path, sha256, root, last_indexed)` — file-sync state.
- `notes(id, kind, tier, title, body, source_path, supersedes, superseded)`.
- `chunks(note_id, ord, text, search_tsv)` — the FTS unit.
- `links(note_id, target_note_id)` — optional associations.

## License

Proprietary and confidential. Copyright (c) Brian Colinger. All rights reserved.
See [LICENSE](LICENSE).
