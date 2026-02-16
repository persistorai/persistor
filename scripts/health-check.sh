#!/usr/bin/env bash
# Persistor - Health Check
set -euo pipefail

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-3030}"
URL="http://${HOST}:${PORT}/api/v1/health"

response=$(curl -sf --max-time 5 "${URL}" 2>/dev/null) || {
    echo "FAIL: service unreachable at ${URL}" >&2
    exit 1
}

status=$(echo "${response}" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)

if [ "${status}" = "ok" ]; then
    echo "OK: ${response}"
    exit 0
else
    echo "DEGRADED: ${response}" >&2
    exit 1
fi
