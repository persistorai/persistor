#!/usr/bin/env bash
# verify.sh — exercise the Claude Code hooks standalone (no live session needed).
#
# Checks:
#   1. pre-compact-nudge.sh emits valid JSON with a top-level systemMessage.
#   2. session-end.sh enqueues a transcript line to an isolated queue file.
#   3. session-start.sh runs `persistor brief` and emits a working-set block
#      (only if DATABASE_URL + PERSISTOR_TENANT_ID + PERSISTOR_NOTES_DIR are set
#      and `persistor` is on PATH; otherwise this check is skipped, not failed).
#
# Exit 0 = all run checks passed.
set -uo pipefail
cd "$(dirname "$0")" || exit 2
fail=0
ok()   { echo "  ok: $*"; }
bad()  { echo "  FAIL: $*"; fail=1; }

command -v jq >/dev/null 2>&1 || { echo "jq required"; exit 2; }

echo "1. pre-compact-nudge.sh"
out=$(printf '{"hook_event_name":"PreCompact","trigger":"auto"}' | ./hooks/pre-compact-nudge.sh)
if jq -e '.systemMessage | type == "string" and length > 0' >/dev/null 2>&1 <<<"$out"; then
  ok "valid JSON with systemMessage"
else
  bad "no systemMessage in: $out"
fi
if jq -e 'has("hookSpecificOutput")' >/dev/null 2>&1 <<<"$out"; then
  bad "uses hookSpecificOutput (rejected for PreCompact)"
else
  ok "no rejected hookSpecificOutput field"
fi

echo "2. session-end.sh"
q="$(mktemp -d)/queue.jsonl"
printf '{"transcript_path":"/tmp/t.jsonl","cwd":"/home/x","session_id":"abc","reason":"clear"}' \
  | PERSISTOR_QUEUE="$q" ./hooks/session-end.sh
if [ -f "$q" ] && jq -e '.transcript_path == "/tmp/t.jsonl" and .session_id == "abc" and (.ts|type=="string")' >/dev/null 2>&1 <"$q"; then
  ok "enqueued transcript with timestamp"
else
  bad "queue line wrong: $(cat "$q" 2>/dev/null)"
fi
rm -rf "$(dirname "$q")"

echo "3. session-start.sh"
if command -v persistor >/dev/null 2>&1 && [ -n "${DATABASE_URL:-}" ] && [ -n "${PERSISTOR_TENANT_ID:-}" ] && [ -n "${PERSISTOR_NOTES_DIR:-}" ]; then
  out=$(printf '{"cwd":"%s","source":"startup"}' "${PERSISTOR_NOTES_DIR}" | ./hooks/session-start.sh)
  if grep -q "Memory working set" <<<"$out"; then
    ok "emitted working-set block"
  else
    bad "no working-set block (got ${#out} bytes)"
  fi
else
  echo "  skip: set DATABASE_URL + PERSISTOR_TENANT_ID + PERSISTOR_NOTES_DIR and put persistor on PATH to check"
fi

[ "$fail" -eq 0 ] && echo "VERIFY: GREEN" || echo "VERIFY: RED"
exit "$fail"
