#!/usr/bin/env bash
# backup-prod.sh — nightly encrypted off-site backup of the production DB.
#
# Why this exists: DO's managed-PG backups live INSIDE the DO account. If the
# account is lost/compromised, so are they. This pulls the data OFF DigitalOcean
# to devbox, encrypted, so account loss is survivable.
#
# What it captures, per tenant: every current note as portable .md via
# `persistor export` (the unit `persistor import` restores), plus a CSV of the
# RLS-exempt tenants/identities routing tables. Schema is NOT dumped — it is
# reproducible from migrations. Export runs tenant-scoped through the store, so
# RLS is satisfied; a naive pg_dump as a non-BYPASSRLS role would silently dump
# ZERO rows from every RLS table and produce a confident-looking empty backup.
#
# Mechanics: opens a short DB-firewall window for this host's public IP
# (removed on exit, even on failure), exports, tars, encrypts with age to the
# the operator backup public key, prunes to the newest 14. Restore procedure:
# deploy/do/README.md "Restore from backup".
set -euo pipefail

DB_ID=8ba2ee1e-1ff8-4b60-a13d-2614ac3e0e6d
BACKUP_DIR=${PERSISTOR_BACKUP_DIR:-/mnt/storage/backups/persistor}
KEEP=14
PUBKEY_FILE="${PERSISTOR_SECRETS:-$HOME/.persistor/secrets}/backup-pubkey.txt"
PERSISTOR_BIN=${PERSISTOR_BIN:-$HOME/.local/bin/persistor}

say() { echo "[backup] $*"; }

command -v age >/dev/null || { echo "FATAL: age not installed" >&2; exit 1; }
command -v doctl >/dev/null || { echo "FATAL: doctl not installed" >&2; exit 1; }
[ -x "$PERSISTOR_BIN" ] || { echo "FATAL: persistor CLI not found at $PERSISTOR_BIN" >&2; exit 1; }
[ -r "$PUBKEY_FILE" ] || { echo "FATAL: age public key not found at $PUBKEY_FILE" >&2; exit 1; }

say "authenticating (Vault -> DO token + DB password, in-memory only)"
# shellcheck disable=SC1091
source "${PERSISTOR_SECRETS:-$HOME/.persistor/secrets}/vault.env"
DIGITALOCEAN_ACCESS_TOKEN=$(vault kv get -field=token secret/digitalocean/pat)
export DIGITALOCEAN_ACCESS_TOKEN
APP_PW=$(vault kv get -field=password secret/persistor/db/app)

CONN=$(doctl databases connection "$DB_ID" -o json)
HOST=$(echo "$CONN" | jq -r .host)
PORT=$(echo "$CONN" | jq -r .port)
DATABASE_URL="postgres://persistor_app:${APP_PW}@${HOST}:${PORT}/persistor?sslmode=require"
export DATABASE_URL

MYIP=$(curl -fsS -m 10 https://api.ipify.org)
[ -n "$MYIP" ] || { echo "FATAL: could not determine public IP" >&2; exit 1; }

remove_firewall() {
  # Remove ONLY the rule we added (uuid captured after append).
  if [ -n "${RULE_UUID:-}" ]; then
    doctl databases firewalls remove "$DB_ID" --uuid "$RULE_UUID" >/dev/null 2>&1 \
      && say "firewall window closed" || say "WARN: failed to remove firewall rule $RULE_UUID — remove manually"
  fi
}
trap remove_firewall EXIT

say "opening DB firewall window for $MYIP"
doctl databases firewalls append "$DB_ID" --rule "ip_addr:$MYIP" >/dev/null
RULE_UUID=$(doctl databases firewalls list "$DB_ID" -o json \
  | jq -r --arg ip "$MYIP" '.[] | select(.type == "ip_addr" and .value == $ip) | .uuid' | head -1)
[ -n "$RULE_UUID" ] || { echo "FATAL: appended firewall rule but could not find its uuid" >&2; exit 1; }

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"; remove_firewall' EXIT

say "routing tables (tenants/identities)"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q \
  -c "\copy (SELECT * FROM tenants) TO '$WORK/tenants.csv' WITH CSV HEADER" \
  -c "\copy (SELECT * FROM identities) TO '$WORK/identities.csv' WITH CSV HEADER"

TENANTS=$(psql "$DATABASE_URL" -tA -c "SELECT id FROM tenants")
COUNT=0
for t in $TENANTS; do
  say "exporting tenant $t"
  "$PERSISTOR_BIN" export --tenant "$t" --out "$WORK/tenant-$t"
  COUNT=$((COUNT + 1))
done
say "exported $COUNT tenant(s)"

mkdir -p "$BACKUP_DIR"
OUT="$BACKUP_DIR/persistor-prod-$STAMP.tar.age"
tar -C "$WORK" -cf - . | age -R "$PUBKEY_FILE" > "$OUT"
say "wrote $OUT ($(du -h "$OUT" | cut -f1))"

# Prune to the newest $KEEP.
ls -1t "$BACKUP_DIR"/persistor-prod-*.tar.age 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
  rm -f "$old" && say "pruned $old"
done

say "OK"
