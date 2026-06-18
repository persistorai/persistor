#!/usr/bin/env bash
# PreCompact hook: remind the model to flush durable memory before context
# compacts. Exit 0 always — never block compaction.
#
# NOTE: emit a top-level `systemMessage`, NOT hookSpecificOutput.additionalContext.
# Claude Code's hook-output schema does not accept additionalContext for the
# PreCompact event (it is rejected as "Invalid input"); systemMessage is the
# supported channel for feedback on this event.
set -uo pipefail

msg="Context is about to compact. If this session produced durable facts, decisions, or user preferences not yet saved, save them NOW via the persistor memory_write MCP tool (with a relative .md path + body) or the auto-memory directory — anything unsaved may be lost in summarization."

jq -n --arg m "$msg" '{systemMessage: $m}' 2>/dev/null || printf '{"systemMessage":%s}\n' "\"$msg\""
exit 0
