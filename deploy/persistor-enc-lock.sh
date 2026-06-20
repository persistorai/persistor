#!/usr/bin/env bash
# Stop the daemon, stop the encrypted Persistor cluster, unmount, and lock the
# LUKS volume. After this the raw container (volume.img) is opaque ciphertext —
# what protects the memory at rest. Requires sudo.
set -euo pipefail

MAP=persistor-enc
MNT=/mnt/persistor-enc
CLUSTER_VER=18
CLUSTER_NAME=persistorenc

sudo systemctl stop persistor-server || true
sudo pg_ctlcluster "$CLUSTER_VER" "$CLUSTER_NAME" stop || true
if mountpoint -q "$MNT"; then
  sudo umount "$MNT"
fi
if [ -e "/dev/mapper/$MAP" ]; then
  sudo cryptsetup close "$MAP"
fi

echo "persistor encrypted instance is locked"
