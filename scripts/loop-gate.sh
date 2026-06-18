#!/usr/bin/env bash
# loop-gate.sh — local build gate.
#
# After the P2.5 slim-down the gate is straightforward: build, vet, lint, then
# tests. The only wrinkle is that the fresh-migration test drops+recreates every
# table, so it runs first and alone (the integration tests in internal/index
# assume the schema exists and would race a concurrent table drop).
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

PGPW="$(cat /tmp/.pgpw_persistor 2>/dev/null)"
if [ -z "${PGPW}" ]; then
  echo "FATAL: /tmp/.pgpw_persistor not found (test DB password)" >&2
  exit 2
fi
export TEST_DATABASE_URL="postgres://persistor:${PGPW}@localhost:5432/persistor_test?sslmode=disable"
export TEST_MIGRATE_DATABASE_URL="${TEST_DATABASE_URL}"

fail=0
step() { echo "=== $* ==="; }

step "go build"
go build ./... || fail=1

step "go vet"
go vet ./... || fail=1

step "golangci-lint"
~/go/bin/golangci-lint run ./... || fail=1

step "fresh migrations (alone, sets schema)"
go test ./internal/db/ -run TestRunMigrationsFreshDatabase -count=1 || fail=1

step "tests (everything except internal/db, which just dropped/rebuilt the schema)"
PKGS="$(go list ./... | grep -v '/internal/db$')"
# shellcheck disable=SC2086
go test -count=1 ${PKGS} || fail=1

if [ "${fail}" -eq 0 ]; then
  echo "GATE: GREEN"
else
  echo "GATE: RED"
fi
exit "${fail}"
