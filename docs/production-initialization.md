# BrickPlans 上线初始化手册

本文档用于把 BrickPlans 从开发/测试环境切换为可上线的空生产环境。

核心原则：**当前环境中的用户、作品、互动、评论、上传图片都是测试数据，正式上线不能保留。** 初始化必须先备份，再生成干净生产库，再创建管理员账号和首批真实内容。

## 1. 初始化前检查

```bash
cd /home/ubuntu/project/brickplans
git status --short --branch
systemctl is-active brickplans-backend || true
curl -s http://127.0.0.1:8100/api/health || true
```

确认事项：

- 代码已切到 `release/v1.0.0` 或对应上线 commit。
- `backend/.env` 已配置生产 `SECRET_KEY`，不能使用默认值。
- 已确认生产域名、端口、HTTPS、备份目录。
- 已准备至少 `10-20` 个真实首发作品素材，避免空站上线。

## 2. 必做备份

即使是测试数据，也先备份，方便回溯和误操作恢复：

```bash
cd /home/ubuntu/project/brickplans
chmod +x scripts/backup_brickplans.sh
./scripts/backup_brickplans.sh
```

校验最新备份：

```bash
BACKUP=$(ls -t backups/brickplans-*.tar.gz | head -1)
sha256sum -c "$BACKUP.sha256"
```

如果服务器没有 `sqlite3` 命令，需要先安装或使用 Python 做库检查；生产建议安装 `sqlite3`，因为备份脚本依赖它的 `.backup` 能力。

## 3. 停止后端

```bash
sudo systemctl stop brickplans-backend
systemctl is-active brickplans-backend || true
```

## 4. 生成干净生产数据库

> 下面步骤会替换运行态数据库和上传目录。执行前必须完成第 2 步备份，并由负责人确认。

为了避免直接删除文件，推荐用归档方式隔离测试数据：

```bash
cd /home/ubuntu/project/brickplans
STAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p runtime-archives/$STAMP

if [ -f backend/brickplans.db ]; then
  mv backend/brickplans.db runtime-archives/$STAMP/brickplans.test.db
fi

if [ -d backend/uploads ]; then
  mv backend/uploads runtime-archives/$STAMP/uploads.test
fi

mkdir -p backend/uploads
```

启动后端，让应用自动创建空表：

```bash
sudo systemctl start brickplans-backend
curl -s http://127.0.0.1:8100/api/health
```

确认新库为空：

```bash
cd /home/ubuntu/project/brickplans
python3 - <<'PY'
import sqlite3
from pathlib import Path
path = Path('backend/brickplans.db')
conn = sqlite3.connect(path)
for table in ['users', 'blueprints', 'blueprint_images', 'comments', 'favorites', 'likes', 'reports', 'notifications']:
    try:
        count = conn.execute(f'select count(*) from {table}').fetchone()[0]
        print(f'{table}: {count}')
    except sqlite3.OperationalError as exc:
        print(f'{table}: missing ({exc})')
conn.close()
PY
```

预期：所有业务表为 `0`。如果表不存在，检查后端日志：

```bash
journalctl -u brickplans-backend -n 100 --no-pager
```

## 5. 创建生产管理员账号

先用接口注册账号：

```bash
curl -s -X POST http://127.0.0.1:8310/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","email":"admin@example.com","password":"CHANGE_ME_STRONG_PASSWORD"}'
```

把该用户提升为管理员：

```bash
cd /home/ubuntu/project/brickplans
python3 - <<'PY'
import sqlite3
email = 'admin@example.com'
conn = sqlite3.connect('backend/brickplans.db')
cur = conn.execute('update users set is_admin = 1 where email = ?', (email,))
conn.commit()
print(f'updated_admin_rows={cur.rowcount}')
conn.close()
PY
```

注意：上面示例里的邮箱和密码必须替换为真实生产管理员信息。执行后立即登录后台验证。

如果输出 `updated_admin_rows=0`，说明邮箱不匹配或注册失败，先不要继续上线。

## 6. 导入首批真实内容

推荐上线前至少准备：

- `10-20` 个真实作品。
- 每个作品至少 `1` 张清晰封面图。
- 标题、描述、难度、零件数、分类、标签完整。
- 管理员审核后再公开。

不要运行 `backend/seed.py` 作为生产首发内容来源；它只适合本地测试。

## 7. 前端构建与静态验证

```bash
cd /home/ubuntu/project/brickplans/frontend
npm ci
npm run build

cd /home/ubuntu/project/brickplans
curl -s -o /dev/null -w 'home HTTP %{http_code}\n' http://127.0.0.1:8310/
curl -s -o /dev/null -w 'api HTTP %{http_code}\n' http://127.0.0.1:8310/api/health
```

如浏览器仍显示旧页面，使用 `Ctrl+Shift+R` 强刷或无痕窗口验证。

## 8. 上线验收清单

- [ ] `backend/.env` 使用生产 `SECRET_KEY`。
- [ ] 测试数据库和测试上传文件已归档隔离。
- [ ] 新生产库业务表为空，或只包含真实管理员/真实作品。
- [ ] 管理员账号可登录后台。
- [ ] 首页、发现页、详情页、注册登录、上传页可访问。
- [ ] `/api/health` 直连和 nginx 代理都正常。
- [ ] `/uploads/` 图片能通过 nginx 访问。
- [ ] 备份脚本可执行，且备份能通过 checksum 校验。
- [ ] 域名、HTTPS、robots、隐私政策页面已确认。
- [ ] 首批真实作品已上传并审核。

## 9. 上线后第一天观察

```bash
journalctl -u brickplans-backend -f
```

重点观察：

- 注册/登录错误。
- 上传失败或 `/uploads/` 404。
- 500 错误。
- SQLite 锁等待或磁盘空间不足。
- 真实用户反馈入口是否可用。

## 10. 回滚原则

- 代码回滚按 `docs/deployment.md`。
- 数据回滚按 `docs/backup.md`。
- 不要手工删除生产数据；所有清理动作都先备份、再归档、再替换。
