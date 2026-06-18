#!/usr/bin/env bash
# SessionStart hook: inject the memory working-set as session context.
#
# Claude Code passes hook JSON on stdin ({cwd, session_id, source, ...}); for
# SessionStart, plain stdout is injected into the session context. This hook MUST
# never fail or block startup: any problem -> exit 0 with no output.
#
# `persistor brief` is self-degrading — if the index/DB is unreachable it reads
# the Core notes straight from disk — so this stays useful even when Postgres is
# down. Env (DATABASE_URL, PERSISTOR_TENANT_ID, PERSISTOR_NOTES_DIR) comes from the
# environment Claude Code launches the hook in.
set -uo pipefail

# Self-source the shared env (DATABASE_URL, PERSISTOR_TENANT_ID, PERSISTOR_NOTES_DIR, PATH)
# so the hook works regardless of how Claude Code launches it. Harmless if absent.
[ -f "$HOME/.persistor/env" ] && . "$HOME/.persistor/env"

input=$(cat 2>/dev/null || true)
cwd=$(jq -r '.cwd // empty' <<<"$input" 2>/dev/null || true)

command -v persistor >/dev/null 2>&1 || exit 0
persistor brief ${cwd:+--cwd "$cwd"} 2>/dev/null || true
exit 0
