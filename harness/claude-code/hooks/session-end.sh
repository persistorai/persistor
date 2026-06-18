#!/usr/bin/env bash
# SessionEnd hook: append this session's transcript to the consolidation queue.
#
# The queue is drained later by the /consolidate skill — NO LLM work here;
# hooks must be fast and free. Each line records the transcript path plus context
# so consolidation can replay it. Never blocks: any problem -> exit 0.
#
# Queue path: $PERSISTOR_QUEUE, else ~/.persistor/consolidation-queue.jsonl
set -uo pipefail

# Self-source the shared env (e.g. PERSISTOR_QUEUE) if present. Harmless if absent.
[ -f "$HOME/.persistor/env" ] && . "$HOME/.persistor/env"

input=$(cat 2>/dev/null || true)
[ -n "$input" ] || exit 0

queue="${PERSISTOR_QUEUE:-$HOME/.persistor/consolidation-queue.jsonl}"
mkdir -p "$(dirname "$queue")" 2>/dev/null || exit 0

jq -c --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{transcript_path: .transcript_path, cwd: .cwd, session_id: .session_id, reason: (.reason // .end_reason // null), ts: $ts}' \
  <<<"$input" >> "$queue" 2>/dev/null || true
exit 0
