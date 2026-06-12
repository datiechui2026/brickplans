# BrickPlans 备份与恢复

BrickPlans 当前生产数据由两部分组成：

1. SQLite 数据库：`backend/brickplans.db`
2. 用户上传文件：`backend/uploads/`

这两类都是运行态数据，不进 git，必须独立备份。

## 手动备份

```bash
cd /home/ubuntu/project/brickplans
chmod +x scripts/backup_brickplans.sh
./scripts/backup_brickplans.sh
```

脚本输出：

- `backups/brickplans-YYYYMMDD-HHMMSS.tar.gz`
- `backups/brickplans-YYYYMMDD-HHMMSS.tar.gz.sha256`

脚本行为：

- 使用 `sqlite3 .backup` 生成一致性 SQLite 备份。
- 打包 `backend/uploads/`。
- 写入 `manifest.txt`，记录时间、路径和 git commit。
- 不自动删除旧备份。

## 自定义备份目录

```bash
BACKUP_DIR=/data/backups/brickplans ./scripts/backup_brickplans.sh
```

可覆盖变量：

| 变量 | 默认值 |
|---|---|
| `APP_DIR` | `/home/ubuntu/project/brickplans` |
| `BACKUP_DIR` | `$APP_DIR/backups` |
| `DB_PATH` | `$APP_DIR/backend/brickplans.db` |
| `UPLOADS_DIR` | `$APP_DIR/backend/uploads` |

## 定时备份建议

每天凌晨 03:20 生成一份备份：

```cron
20 3 * * * cd /home/ubuntu/project/brickplans && /home/ubuntu/project/brickplans/scripts/backup_brickplans.sh >> /home/ubuntu/project/brickplans/backups/backup.log 2>&1
```

注意：旧备份清理涉及删除文件，需要人工确认保留策略后再加清理脚本。

## 恢复流程

假设备份文件是：

```bash
BACKUP=/home/ubuntu/project/brickplans/backups/brickplans-20260612-230000.tar.gz
RESTORE_DIR=/tmp/brickplans-restore
```

解包：

```bash
mkdir -p "$RESTORE_DIR"
tar -xzf "$BACKUP" -C "$RESTORE_DIR"
ls -la "$RESTORE_DIR"
```

校验：

```bash
sha256sum -c "$BACKUP.sha256"
sqlite3 "$RESTORE_DIR/brickplans.db" 'PRAGMA integrity_check;'
```

恢复数据库和上传文件：

```bash
sudo systemctl stop brickplans-backend
cp "$RESTORE_DIR/brickplans.db" /home/ubuntu/project/brickplans/backend/brickplans.db
mkdir -p /home/ubuntu/project/brickplans/backend/uploads
tar -xzf "$RESTORE_DIR/uploads.tar.gz" -C /home/ubuntu/project/brickplans/backend/uploads
sudo systemctl start brickplans-backend
curl http://127.0.0.1:8100/api/health
```

## 恢复演练

每次上线前至少做一次非覆盖式演练：

```bash
BACKUP=$(ls -t backups/brickplans-*.tar.gz | head -1)
RESTORE_DIR=/tmp/brickplans-restore-check
mkdir -p "$RESTORE_DIR"
tar -xzf "$BACKUP" -C "$RESTORE_DIR"
sqlite3 "$RESTORE_DIR/brickplans.db" 'PRAGMA integrity_check;'
```

输出 `ok` 才算备份可用。
