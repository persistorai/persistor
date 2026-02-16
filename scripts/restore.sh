#!/usr/bin/env bash
# Persistor - Database Restore
set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <backup-file> [database-name]" >&2
    exit 1
fi

BACKUP_FILE="$1"
DB_NAME="${2:-persistor}"
AGE_IDENTITY="${AGE_IDENTITY:-$HOME/.config/age/identity.txt}"

echo "[restore] Restoring ${DB_NAME} from ${BACKUP_FILE}"

# Cleanup decrypted file on any exit.
RESTORE_FILE="${BACKUP_FILE}"
cleanup_decrypted() {
    if [[ "${BACKUP_FILE}" == *.age ]] && [ -f "${RESTORE_FILE}" ]; then
        shred -u "${RESTORE_FILE}" 2>/dev/null || rm -f "${RESTORE_FILE}"
    fi
}
trap cleanup_decrypted EXIT ERR

# Verify checksum of encrypted file BEFORE decryption.
if [[ "${BACKUP_FILE}" == *.age ]] && [ -f "${BACKUP_FILE}.sha256" ]; then
    sha256sum -c "${BACKUP_FILE}.sha256"
    echo "[restore] Encrypted file checksum verified"
elif [[ "${BACKUP_FILE}" != *.age ]] && [ -f "${BACKUP_FILE}.sha256" ]; then
    # Verify plaintext checksum if the file is not encrypted.
    sha256sum -c "${BACKUP_FILE}.sha256"
    echo "[restore] Checksum verified"
fi

# Decrypt if needed.
if [[ "${BACKUP_FILE}" == *.age ]]; then
    RESTORE_FILE="${BACKUP_FILE%.age}"
    age -d -i "${AGE_IDENTITY}" -o "${RESTORE_FILE}" "${BACKUP_FILE}"
    echo "[restore] Decrypted"
fi

# Restore.
pg_restore --clean --if-exists --single-transaction -d "${DB_NAME}" "${RESTORE_FILE}"
echo "[restore] Database restored"

echo "[restore] Done"
