#!/usr/bin/env bash
# Persistor - Database Backup
# pg_dump + verify + encrypt + rotate
set -euo pipefail

DB_NAME="${DB_NAME:-persistor}"
BACKUP_DIR="${BACKUP_DIR:-$HOME/backups/persistor}"
AGE_RECIPIENTS="${AGE_RECIPIENTS:-$HOME/.config/age/recipients.txt}"
DATE=$(date +%Y-%m-%d)
DAY_OF_WEEK=$(date +%u)
DAY_OF_MONTH=$(date +%d)
BACKUP_FILE="/dev/shm/persistor-backup-${DATE}.dump"

mkdir -p "${BACKUP_DIR}"/{daily,weekly,monthly}

# Clean up plaintext backup on exit/error.
PLAINTEXT_FILE="${BACKUP_FILE}"
cleanup_plaintext() {
    if [ -f "${PLAINTEXT_FILE}" ]; then
        shred -u "${PLAINTEXT_FILE}" 2>/dev/null || rm -f "${PLAINTEXT_FILE}"
    fi
}
trap cleanup_plaintext EXIT ERR

echo "[backup] Starting backup of ${DB_NAME} at $(date -Iseconds)"

# 1. pg_dump with custom format + compression.
# Uses postgres superuser to bypass RLS; falls back to current user if sudo unavailable.
if sudo -n -u postgres true 2>/dev/null; then
    sudo -u postgres pg_dump --format=custom --compress=9 --no-owner --no-privileges "${DB_NAME}" > "${BACKUP_FILE}"
else
    pg_dump --format=custom --compress=9 --no-owner --no-privileges "${DB_NAME}" > "${BACKUP_FILE}"
fi
echo "[backup] Dump complete: ${BACKUP_FILE} ($(du -h "${BACKUP_FILE}" | cut -f1))"

# 2. Verify dump is parseable.
if ! pg_restore --list "${BACKUP_FILE}" > /dev/null 2>&1; then
    echo "[backup] ERROR: Backup verification failed!" >&2
    exit 1
fi
echo "[backup] Verification passed"

# 3. Encrypt with age.
if [ ! -f "${AGE_RECIPIENTS}" ]; then
    echo "[backup] FATAL: No age recipients file at ${AGE_RECIPIENTS}, aborting" >&2
    exit 1
fi
ENCRYPTED_FILE="${BACKUP_DIR}/daily/backup-${DATE}.dump.age"
age -R "${AGE_RECIPIENTS}" -o "${ENCRYPTED_FILE}" "${BACKUP_FILE}"
shred -u "${BACKUP_FILE}" 2>/dev/null || rm -f "${BACKUP_FILE}"
BACKUP_FILE="${ENCRYPTED_FILE}"
echo "[backup] Encrypted"

# 4. SHA-256 checksum of the encrypted file.
sha256sum "${BACKUP_FILE}" > "${BACKUP_FILE}.sha256"
echo "[backup] Checksum written"

# 5. Weekly copy (Sundays).
if [ "${DAY_OF_WEEK}" = "7" ]; then
    cp "${BACKUP_FILE}" "${BACKUP_DIR}/weekly/"
    echo "[backup] Weekly copy created"
fi

# 6. Monthly copy (1st of month).
if [ "${DAY_OF_MONTH}" = "01" ]; then
    cp "${BACKUP_FILE}" "${BACKUP_DIR}/monthly/"
    echo "[backup] Monthly copy created"
fi

# 7. Rotation: keep 7 daily, 4 weekly, 12 monthly.
find "${BACKUP_DIR}/daily/" -type f -mtime +7 -delete 2>/dev/null || true
find "${BACKUP_DIR}/weekly/" -type f -mtime +28 -delete 2>/dev/null || true
find "${BACKUP_DIR}/monthly/" -type f -mtime +365 -delete 2>/dev/null || true

echo "[backup] Rotation complete"
echo "[backup] Done at $(date -Iseconds)"
