#!/usr/bin/env bash
# gate.sh — the full pre-commit/pre-deploy build gate, self-contained.
#
# Unlike loop-gate.sh (which expects a pre-provisioned local Postgres), this
# script owns its test database: it starts a disposable postgres:18 container
# (127.0.0.1:5467) with the CI role posture (NOSUPERUSER NOBYPASSRLS app role,
# btree_gin) so the RLS/tenant-isolation integration tests ALWAYS run — a bare
# `go test ./...` silently skips them, which is exactly the failure mode this
# gate exists to prevent.
#
# Usage: scripts/gate.sh            # run everything
#        scripts/gate.sh --fresh    # recreate the test DB container first
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

CONTAINER=persistor-test-pg
PORT=5467
URL="postgres://persistor_app:persistor_app@127.0.0.1:${PORT}/persistor_test?sslmode=disable"

if [ "${1:-}" = "--fresh" ]; then
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
fi

ensure_db() {
  if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    docker run -d --name "$CONTAINER" \
      -e POSTGRES_USER=persistor -e POSTGRES_PASSWORD=persistor \
      -e POSTGRES_DB=persistor_test \
      -p "127.0.0.1:${PORT}:5432" postgres:18 >/dev/null || return 1
    # Wait for Postgres to accept connections, then provision the CI role posture.
    for _ in $(seq 1 30); do
      docker exec "$CONTAINER" pg_isready -U persistor -d persistor_test >/dev/null 2>&1 && break
      sleep 1
    done
    docker exec "$CONTAINER" psql -U persistor -d persistor_test -v ON_ERROR_STOP=1 -c \
      "CREATE EXTENSION IF NOT EXISTS btree_gin;
       CREATE ROLE persistor_app LOGIN PASSWORD 'persistor_app' NOSUPERUSER NOBYPASSRLS;
       ALTER SCHEMA public OWNER TO persistor_app;" >/dev/null || return 1
  fi
  return 0
}

fail=0
step() { echo "=== $* ==="; }

step "test database (${CONTAINER} on :${PORT})"
if ! ensure_db; then
  echo "FATAL: could not provision the test database container" >&2
  exit 2
fi
export TEST_DATABASE_URL="$URL"
export TEST_MIGRATE_DATABASE_URL="$URL"

step "go build"
go build ./... || fail=1

step "go vet"
go vet ./... || fail=1

step "golangci-lint"
"$HOME/go/bin/golangci-lint" run ./... || fail=1

# The fresh-migration test drops + recreates every table, so it runs first and
# alone; the other integration tests assume the schema exists.
step "fresh migrations (alone, sets schema)"
go test ./internal/db/ -run TestRunMigrationsFreshDatabase -count=1 || fail=1

step "tests (everything except internal/db, which just rebuilt the schema)"
PKGS="$(go list ./... | grep -v '/internal/db$')"
# shellcheck disable=SC2086
go test -count=1 ${PKGS} || fail=1

if [ "${fail}" -eq 0 ]; then
  echo "GATE: GREEN"
else
  echo "GATE: RED"
fi
exit "${fail}"
