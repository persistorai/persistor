# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-06-18

Memory for AI agents: the agent's markdown notes are the source of truth; a
PostgreSQL full-text index over them makes them searchable.

### Added

- Index (`internal/index`): a content-hash file-sync indexer, markdown frontmatter
  parsing, heading-aware chunking, and FTS retrieval (`SearchNotes`) over Postgres
  `tsvector`.
- Dynamic working-set (`persistor brief`): every pinned Core note plus
  token-budgeted Tail relevant to the current project; degrades to
  Core-read-from-disk when the index is unreachable.
- Consolidation write path (`persistor consolidate`): deterministic plan apply
  that writes prose notes and reconciles supersession; a write whose `supersedes`
  resolves to its own note is rejected.
- Retrieval eval (`internal/eval`): deterministic recall@k against baselines
  (full retrieval / static `MEMORY.md` / no memory), gating every change.
- CLI: `persistor reindex`, `brief`, `search`, `consolidate`, `eval`.
- MCP server (`cmd/persistor-mcp`, stdio): `memory_search`, `memory_get`,
  `memory_write`, `brief`; plus Claude Code hooks and the `/consolidate` skill
  under `harness/claude-code/`.
- Multi-tenant isolation via PostgreSQL row-level security.
