#!/usr/bin/env bash
# Unlock + mount the encrypted Persistor volume, start its Postgres cluster, and
# start the daemon (Phase P4/reliability). Run after a reboot; the cluster is
# manual-start and this unit is not boot-enabled, so nothing on the encrypted
# volume comes up until you run this.
#
# Key source (in order): a local key file if PERSISTOR_LUKS_KEYFILE is set
# (break-glass), otherwise 1Password — the key is streamed straight into
# cryptsetup over stdin and never written to disk. 1Password needs your
# interactive session (`op signin` / unlocked app), which is exactly what keeps a
# stolen powered-off disk unreadable.
set -euo pipefail

IMG=${PERSISTOR_LUKS_IMG:-/var/lib/persistor-enc/volume.img}
MAP=persistor-enc
MNT=/mnt/persistor-enc
CLUSTER_VER=18
CLUSTER_NAME=persistorenc
OP_ITEM=${PERSISTOR_LUKS_OP_ITEM:-Persistor LUKS key (warp-core)}
OP_VAULT=${PERSISTOR_LUKS_OP_VAULT:-Personal}

open_volume() {
  if [ -e "/dev/mapper/$MAP" ]; then
    return 0
  fi
  if [ -n "${PERSISTOR_LUKS_KEYFILE:-}" ]; then
    sudo cryptsetup open --key-file "$PERSISTOR_LUKS_KEYFILE" "$IMG" "$MAP"
  else
    # Stream the key from 1Password straight into cryptsetup; it never lands on disk.
    op document get "$OP_ITEM" --vault "$OP_VAULT" | sudo cryptsetup open --key-file - "$IMG" "$MAP"
  fi
}

open_volume
mountpoint -q "$MNT" || sudo mount "/dev/mapper/$MAP" "$MNT"
sudo pg_ctlcluster "$CLUSTER_VER" "$CLUSTER_NAME" start || true

# Wait for Postgres to actually accept connections before starting the daemon,
# so the daemon's pool connects on the first try instead of crash-looping.
for _ in $(seq 1 30); do
  if pg_isready -h 127.0.0.1 -p 5434 -q; then break; fi
  sleep 0.5
done
# start (not restart): a no-op if already healthy, so re-running never bounces a
# live daemon; the pg_isready wait above means a fresh start connects first try.
sudo systemctl start persistor-server

echo "persistor encrypted instance + daemon are up (:5434 DB, :8088 MCP)"
