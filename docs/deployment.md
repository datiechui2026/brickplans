# BrickPlans 生产部署手册

本文档用于把 BrickPlans 部署到生产服务器。当前推荐架构：FastAPI 后端由 systemd 管理，Vite 前端构建产物由 nginx 静态托管，`/api/` 与 `/uploads/` 反向代理到后端。

> 生产上线前必须先执行 `docs/production-initialization.md`。当前开发环境内的用户、作品、上传文件都是测试数据，不能直接带到生产。

## 1. 部署拓扑

| 项 | 默认值 |
|---|---|
| 项目目录 | `/home/ubuntu/project/brickplans` |
| 后端服务 | `brickplans-backend` |
| 后端监听 | `127.0.0.1:8100` |
| 前端监听 | `0.0.0.0:8310` |
| 前端静态目录 | `/home/ubuntu/project/brickplans/frontend/dist` |
| SQLite 数据库 | `/home/ubuntu/project/brickplans/backend/brickplans.db` |
| 图片存储 | 默认本地 `/home/ubuntu/project/brickplans/backend/uploads/`；生产推荐 Tencent COS |
| nginx 配置 | `/etc/nginx/sites-available/brickplans` |
| systemd unit | `/etc/systemd/system/brickplans-backend.service` |

生产域名接入时，把 `brickplans-nginx.conf` 里的 `listen 8310; server_name _;` 调整为正式端口/域名，并在外层负载均衡或 nginx 主站上配置 HTTPS。

## 2. 服务器前置检查

```bash
python3 --version
node --version
npm --version
nginx -v
systemctl --version
ss -tlnp | grep -E ':(8100|8310)\b' || true
df -h /
```

要求：

- Ubuntu/Debian 系 Linux。
- Python 3.10+。
- Node.js 18+。
- nginx + systemd 可用。
- `8100`、`8310` 未被其他正式服务占用，或已按生产规划改端口。
- 磁盘空间足够保存 SQLite、上传文件和备份。

## 3. 获取代码

```bash
sudo mkdir -p /home/ubuntu/project
sudo chown -R ubuntu:ubuntu /home/ubuntu/project
cd /home/ubuntu/project

git clone <REPO_URL> brickplans
cd brickplans
git checkout release/v1.0.0
```

如果是从发布包部署：

```bash
mkdir -p /home/ubuntu/project/brickplans
cd /home/ubuntu/project/brickplans
tar -xzf /path/to/brickplans-release-v1.0.0.tar.gz --strip-components=1
```

## 4. 后端部署

创建虚拟环境并安装依赖：

```bash
cd /home/ubuntu/project/brickplans/backend
python3 -m venv .venv
. .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
```

创建生产环境变量文件：

```bash
cd /home/ubuntu/project/brickplans/backend
python3 - <<'PY'
import secrets
print(secrets.token_urlsafe(48))
PY
```

把输出写入 `backend/.env`：

```env
DEBUG=false
DATABASE_URL=sqlite+aiosqlite:////home/ubuntu/project/brickplans/backend/brickplans.db
DATABASE_URL_SYNC=sqlite:////home/ubuntu/project/brickplans/backend/brickplans.db
SECRET_KEY=<上一步生成的长随机字符串>
CORS_ORIGINS=["https://YOUR_DOMAIN"]

# 图片存储：本地模式
STORAGE_BACKEND=local

# 图片存储：腾讯云 COS 模式，启用时把 STORAGE_BACKEND 改成 tencent_cos
# STORAGE_BACKEND=tencent_cos
# TENCENT_COS_SECRET_ID=<COS SecretId>
# TENCENT_COS_SECRET_KEY=<COS SecretKey>
# TENCENT_COS_BUCKET=<bucket-appid，例如 brickplans-1250000000>
# TENCENT_COS_REGION=<地域，例如 ap-guangzhou>
# TENCENT_COS_PUBLIC_BASE_URL=<访问域名，例如 https://brickplans-1250000000.cos.ap-guangzhou.myqcloud.com 或 CDN 域名>
```

启用 COS 后，作品图片会上传到 `blueprints/<uuid>.<ext>`，头像会上传到 `avatars/<uuid>.<ext>`；`blueprint_images.object_key` 保存 COS object key，`blueprint_images.url` 保存前端可访问 URL。

本机 8310 端口试运行时可以临时用：

```env
CORS_ORIGINS=["http://YOUR_SERVER_HOST:8310"]
```

安装并启动 systemd：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-backend.service /etc/systemd/system/brickplans-backend.service
sudo systemctl daemon-reload
sudo systemctl enable --now brickplans-backend
systemctl status brickplans-backend --no-pager
curl http://127.0.0.1:8100/api/health
```

## 5. 前端部署

同源 nginx 部署时，前端 API 走相对路径 `/api`，不要设置 `VITE_API_BASE_URL`：

```bash
cd /home/ubuntu/project/brickplans/frontend
npm ci
npm run build
```

如果前端部署到 Vercel/Cloudflare Pages，需要设置：

```bash
VITE_API_BASE_URL=https://YOUR_API_DOMAIN
```

安装 nginx 配置：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-nginx.conf /etc/nginx/sites-available/brickplans
sudo ln -sf /etc/nginx/sites-available/brickplans /etc/nginx/sites-enabled/brickplans
sudo nginx -t
sudo systemctl reload nginx
```

检查是否有重复监听同一端口：

```bash
sudo nginx -T | grep 'listen 8310' -B1
```

同一个端口只应有一个 BrickPlans server block。若 `conf.d/` 和 `sites-enabled/` 都配置了同端口，需要先人工确认后再移除旧配置。

## 6. 部署验证

```bash
systemctl is-active brickplans-backend
ss -tlnp | grep -E ':(8100|8310)\b'
curl -s http://127.0.0.1:8100/api/health
curl -s http://127.0.0.1:8310/api/health
curl -s -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:8310/
curl -sI http://127.0.0.1:8310/ | head
```

预期：

- systemd 状态为 `active`。
- 后端直连 `/api/health` 返回 `{"status":"ok","version":"0.1.0"}`。
- nginx 代理 `/api/health` 返回同样结果。
- 首页返回 `HTTP 200`。

## 7. 更新部署

```bash
cd /home/ubuntu/project/brickplans
git fetch origin
git checkout release/v1.0.0
git pull --ff-only

cd backend
. .venv/bin/activate
pip install -r requirements.txt
sudo systemctl restart brickplans-backend
curl http://127.0.0.1:8100/api/health

cd ../frontend
npm ci
npm run build
sudo nginx -t
sudo systemctl reload nginx
```

前端确认：

```bash
ls -lt /home/ubuntu/project/brickplans/frontend/dist/assets | head
```

如果浏览器看不到更新，先 `Ctrl+Shift+R` 强刷或用无痕窗口验证。

## 8. 备份与恢复

上线前、每次更新前、每次数据清理前都先备份：

```bash
cd /home/ubuntu/project/brickplans
chmod +x scripts/backup_brickplans.sh
./scripts/backup_brickplans.sh
```

恢复流程见 `docs/backup.md`。

## 9. 回滚

代码回滚：

```bash
cd /home/ubuntu/project/brickplans
git log --oneline -10
git checkout <GOOD_COMMIT_OR_TAG>

cd frontend
npm ci
npm run build

cd ../backend
. .venv/bin/activate
pip install -r requirements.txt
sudo systemctl restart brickplans-backend
sudo nginx -t
sudo systemctl reload nginx
```

数据回滚必须走 `docs/backup.md`，不要直接覆盖生产库。

## 10. 常见问题

- **前端看不到更新**：先强刷；再确认 `frontend/dist/assets/index-*.js` 时间戳是否已更新。
- **`/uploads/` 404**：确认 nginx 使用 `location ^~ /uploads/` 代理到 `127.0.0.1:8100`，避免被图片扩展名 regex 抢走。
- **后端测试找不到依赖**：使用 `backend/.venv/bin/python -m pytest`，不要用系统 Python。
- **生产仍使用默认密钥**：检查 `backend/.env` 里的 `SECRET_KEY`，不能是 `change-me-in-production-use-a-long-random-string`。
- **SQLite 表缺字段**：当前启动时会执行轻量迁移，但复杂 schema 变更仍需上线前演练。
- **误带测试数据**：生产初始化必须执行 `docs/production-initialization.md`，不要直接复制开发库和上传目录。
