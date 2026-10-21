#!/usr/bin/env bash
# Dump Postgres to a local file. The free substitute for RDS automated backups.
#
# The measurement history is the only thing in this project that cannot be
# rebuilt, so it is the only thing worth backing up.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${ROOT}/data/backups"
KEEP="${KEEP:-14}"
CONTAINER="${CONTAINER:-sovereign-db-1}"

mkdir -p "$DEST"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FILE="${DEST}/sovereign-${STAMP}.sql.gz"

docker exec "$CONTAINER" pg_dump -U sovereign -d sovereign --no-owner \
  | gzip > "$FILE"

echo "wrote $FILE ($(du -h "$FILE" | cut -f1))"

# Keep the most recent N, delete the rest.
ls -1t "${DEST}"/sovereign-*.sql.gz 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
  rm -f "$old"
  echo "pruned $(basename "$old")"
done
