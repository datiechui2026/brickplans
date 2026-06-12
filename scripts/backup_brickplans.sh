#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/home/ubuntu/project/brickplans}"
BACKUP_DIR="${BACKUP_DIR:-$APP_DIR/backups}"
DB_PATH="${DB_PATH:-$APP_DIR/backend/brickplans.db}"
UPLOADS_DIR="${UPLOADS_DIR:-$APP_DIR/backend/uploads}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="$BACKUP_DIR/$STAMP"
ARCHIVE="$BACKUP_DIR/brickplans-$STAMP.tar.gz"

mkdir -p "$OUT_DIR"

if [ ! -f "$DB_PATH" ]; then
  echo "Database not found: $DB_PATH" >&2
  exit 1
fi

sqlite3 "$DB_PATH" ".backup '$OUT_DIR/brickplans.db'"

if [ -d "$UPLOADS_DIR" ]; then
  tar -C "$UPLOADS_DIR" -czf "$OUT_DIR/uploads.tar.gz" .
else
  mkdir -p "$OUT_DIR/uploads-empty"
fi

cat > "$OUT_DIR/manifest.txt" <<MANIFEST
created_at=$(date -Is)
app_dir=$APP_DIR
db_path=$DB_PATH
uploads_dir=$UPLOADS_DIR
git_commit=$(git -C "$APP_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)
MANIFEST

tar -C "$OUT_DIR" -czf "$ARCHIVE" .
sha256sum "$ARCHIVE" > "$ARCHIVE.sha256"

echo "Backup created: $ARCHIVE"
echo "Checksum: $ARCHIVE.sha256"
echo "Note: old backups are not deleted automatically."
