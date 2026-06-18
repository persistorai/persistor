# Claude Code harness wiring

Install artifacts that wire the memory engine into a Claude Code session. Nothing
here installs itself — wiring it up is a deliberate, reviewed step.

## Pieces

| Piece                         | What it does                                                                                                                                                             |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `hooks/session-start.sh`      | SessionStart → runs `persistor brief` and injects the bounded working-set (Core + retrieved Tail) as session context. Self-degrades to Core-from-disk if the DB is down. |
| `hooks/session-end.sh`        | SessionEnd → appends the transcript path to the consolidation queue (`~/.persistor/consolidation-queue.jsonl`). No LLM work.                                             |
| `hooks/pre-compact-nudge.sh`  | PreCompact → emits a `systemMessage` nudge to flush durable memory before compaction.                                                                                    |
| `skills/consolidate/SKILL.md` | The `/consolidate` skill: the one LLM-judgment step — read queued transcripts, search-before-write, emit a plan, apply it with `persistor consolidate`.                  |
| `settings.snippet.json`       | The `hooks` block to **merge** into `~/.claude/settings.json`.                                                                                                           |
| `verify.sh`                   | Run the three hooks standalone and check they emit/enqueue correctly.                                                                                                    |

## Install

1. Build the binaries: `go build -o ~/.local/bin/persistor ./cmd/persistor-cli`
   and `go build -o ~/.local/bin/persistor-mcp ./cmd/persistor-mcp`.
2. Put `DATABASE_URL`, `PERSISTOR_TENANT_ID`, and `PERSISTOR_NOTES_DIR` (and
   optionally `CLAUDE_MEMORY_DIR`) in an env file, and add a small wrapper that
   sources it before exec'ing `persistor-mcp` — so no secrets live in the Claude
   Code config:

   ```bash
   cat > ~/.persistor/persistor-mcp.sh <<'EOF'
   #!/usr/bin/env bash
   [ -f "$HOME/.persistor/env" ] && . "$HOME/.persistor/env"
   exec ~/.local/bin/persistor-mcp
   EOF
   chmod +x ~/.persistor/persistor-mcp.sh
   ```

3. Register the MCP server (Claude Code reads `~/.claude.json`, not
   `settings.json`):

   ```bash
   claude mcp add persistor ~/.persistor/persistor-mcp.sh
   claude mcp get persistor          # expect: Status ✔ Connected
   ```

4. Copy the hooks somewhere stable, `chmod +x` them, and merge the `hooks` block
   from `settings.snippet.json` into `~/.claude/settings.json` (pointing at where
   you copied them). The hooks self-source `~/.persistor/env`.
5. Index the notes: `persistor reindex`.

## Verifying without a live session

`./verify.sh` exercises the hooks mechanically (no Claude session needed). The
true live-session check — a fresh Claude Code session actually showing the
working-set and round-tripping `memory_search` — is a manual step.
