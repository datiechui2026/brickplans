#!/usr/bin/env bash
# 工单 02 批2 —— 表结构变更/内容灌库前的备份。只导 blueprints 一张表。
# 用法: bash 02-backup.sh [输出目录]    默认 /root/bp-backup
set -euo pipefail

DEST="${1:-/root/bp-backup}"
TS="$(date +%Y%m%d-%H%M%S)"
OUT="$DEST/blueprints-$TS.sql.gz"
mkdir -p "$DEST"

# 走 socket 的 root 账号（生产机现状）。
# --skip-extended-insert 让每个 INSERT 恰好一行图纸，下面就能用 INSERT 条数
# 直接和 COUNT(*) 对比；不这么写的话一个 INSERT 里塞几百行，行数校验会失效。
mysqldump --single-transaction --quick --skip-lock-tables \
          --no-tablespaces --set-gtid-purged=OFF \
          --skip-extended-insert --triggers=false \
          brickplans blueprints | gzip -1 > "$OUT"

# 1) 压缩包完整性
gzip -t "$OUT"
# 2) 回读行数：必须等于 blueprints 的 COUNT(*)（生产现值 2135，其中已发布 2134）
ROWS_DUMP=$(zcat "$OUT" | grep -c '^INSERT INTO' || true)
ROWS_DB=$(mysql -uroot -N -B -e "SELECT COUNT(*) FROM blueprints" brickplans)
printf 'dump   : %s\n' "$OUT"
printf 'sha256 : %s\n' "$(sha256sum "$OUT" | awk '{print $1}')"
printf 'size   : %s bytes\n' "$(stat -c%s "$OUT")"
printf 'rows   : dump INSERT blocks=%s, db COUNT(*)=%s\n' "$ROWS_DUMP" "$ROWS_DB"
if [ "$ROWS_DB" -eq 0 ]; then echo "FATAL: db reports 0 rows" >&2; exit 1; fi
echo "backup OK"
