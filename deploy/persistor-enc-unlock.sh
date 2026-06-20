#!/usr/bin/env bash
# Unlock + mount the encrypted Persistor volume and start its Postgres cluster
# (the remote-MCP data lives here, encrypted at rest — Phase P4). Run after a
# reboot; the cluster is set to manual start so it never fail-starts on the
# absent mount. Requires sudo.
#
# Key source: PERSISTOR_LUKS_KEY (default /root/.persistor-luks.key). For true
# disk-theft protection the key must NOT live on the same disk — see
# deploy/p4-encryption-tls.md (passphrase prompt / removable media / Vault).
set -euo pipefail

IMG=${PERSISTOR_LUKS_IMG:-/var/lib/persistor-enc/volume.img}
KEY=${PERSISTOR_LUKS_KEY:-/root/.persistor-luks.key}
MAP=persistor-enc
MNT=/mnt/persistor-enc
CLUSTER_VER=18
CLUSTER_NAME=persistorenc

if [ ! -e "/dev/mapper/$MAP" ]; then
  sudo cryptsetup open --key-file "$KEY" "$IMG" "$MAP"
fi
if ! mountpoint -q "$MNT"; then
  sudo mount "/dev/mapper/$MAP" "$MNT"
fi
sudo pg_ctlcluster "$CLUSTER_VER" "$CLUSTER_NAME" start || true

echo "persistor encrypted instance is up on :5434 (data dir $MNT/pgdata)"
