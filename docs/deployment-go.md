# BrickPlan 后端 (Go) 部署手册

本文档部署 Go 版后端 (`backend-go/`，Gin + GORM + MySQL)，替代 Python 版 (`backend/`)。前端与 nginx 配置基本不变：nginx 仍把 `/api/`、`/uploads/` 反代到 `127.0.0.1:8100`，只是后端进程由 Python/uvicorn 换成 Go 二进制，数据库由 SQLite 换成 MySQL。

> 旧的 Python 部署文档见 `docs/deployment.md`，保留作为回退参考。

## 1. 拓扑

| 项 | 默认值 |
|---|---|
| 项目目录 | `/home/ubuntu/project/brickplans` |
| 后端二进制 | `/home/ubuntu/project/brickplans/backend-go/brickplans-backend` |
| 后端服务 | `brickplans-backend-go` (systemd) |
| 后端监听 | `127.0.0.1:8100` |
| 前端监听 | `0.0.0.0:8310` (nginx) |
| MySQL | `127.0.0.1:3306`，库名 `brickplans` |
| 上传目录 | `backend-go/uploads/`（本地存储）或腾讯 COS |
| 配置 | `backend-go/.env` |

## 2. 安装 MySQL 8

```bash
sudo apt update
sudo apt install -y mysql-server
sudo systemctl enable --now mysql
sudo mysql_secure_installation
```

建库建用户：

```sql
CREATE DATABASE brickplans CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'brickplans'@'localhost' IDENTIFIED BY 'CHANGE_ME_STRONG_PASSWORD';
GRANT ALL ON brickplans.* TO 'brickplans'@'localhost';
FLUSH PRIVILEGES;
```

## 3. 安装 Go

```bash
go version           # 需要 1.24+
# 如未安装：
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
```

## 4. 构建后端

```bash
cd /home/ubuntu/project/brickplans/backend-go
go mod download
go build -o brickplans-backend ./cmd/server
```

## 5. 配置 `.env`

```bash
cd /home/ubuntu/project/brickplans/backend-go
cp .env.example .env
# 生成 SECRET_KEY
openssl rand -hex 32
```

编辑 `.env`：

```env
APP_ENV=production
HTTP_ADDR=127.0.0.1:8100
SECRET_KEY=<openssl rand -hex 32 的输出，必须 ≥32 字符>
JWT_ACCESS_MIN=30
JWT_REFRESH_DAYS=7
MYSQL_DSN=brickplans:CHANGE_ME_STRONG_PASSWORD@tcp(127.0.0.1:3306)/brickplans?charset=utf8mb4&parseTime=True&loc=UTC
CORS_ORIGINS=https://YOUR_DOMAIN
STORAGE_BACKEND=local
UPLOAD_DIR=/home/ubuntu/project/brickplans/backend-go/uploads

# 腾讯 COS（可选）
# TENCENT_COS_SECRET_ID=
# TENCENT_COS_SECRET_KEY=
# TENCENT_COS_BUCKET=
# TENCENT_COS_REGION=
# TENCENT_COS_PUBLIC_BASE_URL=
```

> 服务启动时会校验 `SECRET_KEY`：若是内置默认值或长度 < 32，`log.Fatal` 拒绝启动。

## 6. systemd unit

`/etc/systemd/system/brickplans-backend-go.service`：

```ini
[Unit]
Description=BrickPlan Go backend
After=network.target mysql.service

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/project/brickplans/backend-go
EnvironmentFile=/home/ubuntu/project/brickplans/backend-go/.env
ExecStart=/home/ubuntu/project/brickplans/backend-go/brickplans-backend
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用并验证（首次启动会 AutoMigrate 建表）：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now brickplans-backend-go
systemctl status brickplans-backend-go --no-pager
curl http://127.0.0.1:8100/api/health   # {"status":"ok","version":"0.2.0"}
```

如需把官方账号设为管理员，手动执行：

```sql
UPDATE users SET is_admin = 1 WHERE email = 'official@brickplan.cn';
```

## 7. 初始化数据

```bash
cd /home/ubuntu/project/brickplans/backend-go
SEED_ADMIN_PASSWORD=<强密码> go run ./cmd/seed
```

`cmd/seed` 幂等：已存在 ≥10 条作品时跳过。

## 7a. 从旧 Python SQLite 迁移数据（可选）

如果要把旧 `backend/brickplans.db` 的数据搬到 MySQL（而非重新 seed）：

```bash
cd /home/ubuntu/project/brickplans/backend-go
go run ./cmd/migrate --sqlite ../backend/brickplans.db            # 幂等(ON DUPLICATE KEY 跳过)
go run ./cmd/migrate --sqlite ../backend/brickplans.db --reset    # 先清空 MySQL 再导
```

迁移保留 UUID、时间戳、bcrypt 密码哈希（passlib `$2b$` 与 Go bcrypt 二进制兼容）；旧用户标记 `email_verified=true` 以免被 24h 未验证清理删除。上传文件（`backend/uploads/`）需手动复制到 `backend-go/uploads/`，或把 `UPLOAD_DIR` 指向旧目录。

## 7b. 批量翻译存量图纸的英文标题/描述

`title_en` / `description_en` 是可选字段，老数据为空时英文站回退显示中文。要把存量中文标题和描述一次性翻成英文：

```bash
cd /home/ubuntu/project/brickplans/backend-go
go run ./cmd/translate-blueprints --dry-run       # 先看会翻哪些
go run ./cmd/translate-blueprints                 # 翻译所有缺失字段（幂等，可断点续跑）
go run ./cmd/translate-blueprints --limit 20      # 只翻前 20 条
go run ./cmd/translate-blueprints --force         # 重翻已有译文
```

依赖与英文博客改写流水线相同：`ANTHROPIC_API_KEY`（可选 `ANTHROPIC_BASE_URL` / `ANTHROPIC_MODEL`），数据库读 `.env` 的 `MYSQL_DSN`。命令启动时会自动补齐两个新列；已存在译文的行会跳过，失败批次下次重跑即可。建议在低峰执行，`--batch 10` 为默认批量。

## 8. 前端 + nginx

前端构建不变：

```bash
cd /home/ubuntu/project/brickplans/frontend
npm ci && npm run build
```

nginx 配置仍用仓库根的 `brickplans-nginx.conf`（已加入安全响应头 + SSR 反代）。安装：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-nginx.conf /etc/nginx/sites-available/brickplans
sudo ln -sf /etc/nginx/sites-available/brickplans /etc/nginx/sites-enabled/brickplans
sudo nginx -t && sudo systemctl reload nginx
```

> 该 nginx 配置把 `/assets/`、静态扩展名、`/robots.txt`、`/llms.txt`、`/avatars/` 交给 nginx 直接服务；`/api/`、`/uploads/` 反代后端；**其余所有路径(`/`、`/detail/:id`、`/explore`、`/tags/:name`、`/user/:id` 等)反代后端做 SSR**，让搜索引擎和 AI 爬虫拿到服务端渲染的完整 HTML（含 title/meta/JSON-LD/noscript 正文）。后端通过 `FRONTEND_DIST` 读取 `dist/manifest.json` 引用带 hash 的 `/assets/main-<hash>.{js,css}`。`PUBLIC_URL` 决定 canonical/og:url/sitemap 的域名。

如之前运行的是 Python 后端，停掉旧 unit（保留代码以便回退）：

```bash
sudo systemctl disable --now brickplans-backend
```

## 9. 验证

```bash
systemctl is-active brickplans-backend-go
ss -tlnp | grep -E ':(8100|8310)\b'
curl -s http://127.0.0.1:8100/api/health
curl -sI http://127.0.0.1:8310/ | grep -iE 'x-content-type-options|strict-transport'
```

## 10. 更新

**顺序要点：前端必须先构建，后端后重启。** `ssr.NewRenderer` 在**启动时**读
`frontend/dist/manifest.json`（见 `internal/ssr/ssr.go`），而 Vite 每次构建都会换掉带 hash 的
文件名。先重启后端的话，它记住的是**旧 hash**，新构建一落盘旧 hash 就 404，直到你再重启一次
才恢复——用户看到的是"页面没样式/白屏"，爬虫拿到的是坏 HTML。正确顺序：

```bash
cd /home/ubuntu/project/brickplans
git fetch --tags && git checkout -f <tag>      # 生产按 tag 部署

# 1) 前端先构建（SSR 需要新的 manifest.json）
cd frontend && npm ci && npm run build

# 2) 再处理后端：生产 1.87G 内存禁止 go build（OOM），用本机交叉编译产物替换
#    CGO_ENABLED=0 go build -ldflags='-s -w' ./cmd/server  → 传上来
cd ../backend-go
sudo systemctl restart brickplans-backend-go   # 重启时才读 manifest

# 3) nginx 一般无需 reload；改了 nginx 配置才执行
sudo nginx -t && sudo systemctl reload nginx
```

**部署后必须校验 asset hash 一致**（下面的检查一旦报 MISSING，就是"后端比 build 早启动"：

```bash
cd /home/ubuntu/project/brickplans
for a in $(curl -s http://127.0.0.1:8100/ | grep -oE '/assets/[A-Za-z0-9._-]+\.(js|css)' | sort -u); do
  if [ -f "frontend/dist$a" ]; then echo "OK      $a"; else echo "MISSING $a  ← 重启后端再验一次"; fi
done
# 反向再验一次：dist 里最新的 js 是否真被 HTML 引用（防止服务的是上一版）
latest=$(ls -t frontend/dist/assets/*.js | head -1 | sed 's|frontend/dist||')
curl -s http://127.0.0.1:8100/ | grep -q "$latest" && echo "OK      $latest 已被引用" || echo "STALE   前端未生效"
```

## 10a. 英文站放量台账（100 篇/天）

英文 1,536 篇译文由 `content/en/sitemap-exposure.txt` 控制放量（`internal/blog/exposure.go`）：
台账内的 slug 进 `sitemap-en-blog-*.xml`、可索引、可被 `hreflang` 引用、出现在 `/en/` 与
`/en/blog`；未放量的文章仍返回 200，但带 `noindex, follow` + `X-Robots-Tag`，且从列表与
sitemap 里过滤掉。四个面共用一张表，所以不会漂移。

**每天 +100 由脚本执行**（仓库里没有 cron 就永远停在种子批次）：

```bash
# 手动：查看进度 / 模拟 / 真放量
python3 scripts/brickplan_en_sitemap_release.py --status
python3 scripts/brickplan_en_sitemap_release.py --dry-run
python3 scripts/brickplan_en_sitemap_release.py            # 每天第一次运行才生效（幂等），--force 可强制
```

生产把台账放在**仓库外**（`EXPOSURE_LEDGER_PATH`），否则下一次 `git checkout -f <tag>` 会把
放量进度回退到 tag 里那份。`backend-go/.env` 或 systemd unit 里设：

```bash
EXPOSURE_LEDGER_PATH=/home/ubuntu/project/brickplans/shared/sitemap-exposure.txt
```

每日定时（systemd timer，日志进 journal）：

```ini
# /etc/systemd/system/brickplan-en-release.service
[Unit]
Description=brickplan.cn English sitemap exposure release (+100/day)
[Service]
Type=oneshot
WorkingDirectory=/home/ubuntu/project/brickplans
Environment=EXPOSURE_LEDGER_PATH=/home/ubuntu/project/brickplans/shared/sitemap-exposure.txt
ExecStart=/usr/bin/python3 scripts/brickplan_en_sitemap_release.py

# /etc/systemd/system/brickplan-en-release.timer
[Unit]
Description=Daily English sitemap exposure release
[Timer]
OnCalendar=*-*-* 00:15:00 UTC
Persistent=true
[Install]
WantedBy=timers.target
```

`systemctl enable --now brickplan-en-release.timer` 即可；`Persistent=true` 让机器停机后补跑一次。
**回滚**：`rm` 掉台账文件 → 闸门自动失效，全部英文文章恢复收录（改完下个请求生效，无需重启）。

## 11. 回退到 Python 后端

```bash
sudo systemctl disable --now brickplans-backend-go
sudo systemctl enable --now brickplans-backend   # Python 仍指向 127.0.0.1:8100
```

注意 Python 用 SQLite，Go 用 MySQL，数据不互通；回退后需从 SQLite 备份恢复，或重新初始化。见 `docs/deployment.md`。

## 12. 备份

MySQL 备份：

```bash
mysqldump -u brickplans -p brickplans > backup-$(date +%F).sql
```

上传文件备份（本地存储时）：

```bash
tar -czf uploads-$(date +%F).tar.gz backend-go/uploads/
```
