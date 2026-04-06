#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${1:-./backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
ARCHIVE="vppp-backup-${STAMP}.tar.gz"

mkdir -p "$BACKUP_DIR"

echo "[vppp] stopping service for a consistent backup..."
docker compose stop vpn

cleanup() {
  echo "[vppp] starting service again..."
  docker compose start vpn
}
trap cleanup EXIT

echo "[vppp] creating backup archive ${BACKUP_DIR}/${ARCHIVE}..."
docker compose run --rm --no-deps vpn sh -lc "tar czf - /data /etc/wireguard" > "${BACKUP_DIR}/${ARCHIVE}"

echo "[vppp] backup complete: ${BACKUP_DIR}/${ARCHIVE}"
