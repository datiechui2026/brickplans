# BrickPlans 后端 (Go) 部署手册

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
Description=BrickPlans Go backend
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
UPDATE users SET is_admin = 1 WHERE email = 'official@brickplans.com';
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

## 8. 前端 + nginx

前端构建不变：

```bash
cd /home/ubuntu/project/brickplans/frontend
npm ci && npm run build
```

nginx 配置仍用仓库根的 `brickplans-nginx.conf`（已加入安全响应头）。安装：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-nginx.conf /etc/nginx/sites-available/brickplans
sudo ln -sf /etc/nginx/sites-available/brickplans /etc/nginx/sites-enabled/brickplans
sudo nginx -t && sudo systemctl reload nginx
```

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

```bash
cd /home/ubuntu/project/brickplans
git pull --ff-only
cd backend-go && go build -o brickplans-backend ./cmd/server
sudo systemctl restart brickplans-backend-go
cd ../frontend && npm ci && npm run build
sudo systemctl reload nginx
```

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
