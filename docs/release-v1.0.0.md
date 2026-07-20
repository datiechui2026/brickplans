# BrickPlan v1.0.0 Release Checklist

本文档记录 `release/v1.0.0` 的交付范围、验证命令和发布包约定。

## 1. 版本定位

`v1.0.0` 是 BrickPlan 的生产部署准备版本，目标是支持上线 Beta/种子用户运营。

交付重点：

- FastAPI 后端 + SQLite 生产单机部署。
- Vite + Vanilla JS 前端构建和 nginx 托管。
- 用户注册登录、图纸发布、图片管理、搜索浏览、互动、后台审核等 MVP 功能。
- 生产部署文档、备份文档、上线初始化文档。
- 明确测试数据不能进入生产。

## 2. 发布前必须验证

```bash
cd /home/ubuntu/project/brickplans

git status --short --branch
node --check frontend/src/main.js
backend/.venv/bin/python -m pytest -q backend/tests
cd frontend && npm run build
```

部署健康检查：

```bash
cd /home/ubuntu/project/brickplans
systemctl is-active brickplans-backend
curl -s http://127.0.0.1:8100/api/health
curl -s http://127.0.0.1:8310/api/health
curl -s -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:8310/
```

## 3. 发布包内容

发布包应包含：

- 后端源代码：`backend/app/`、`backend/requirements.txt`、`backend/tests/`。
- 前端源代码：`frontend/src/`、`frontend/public/`、`frontend/package*.json`、`frontend/vite.config.js`。
- 部署配置：`brickplans-backend.service`、`brickplans-nginx.conf`。
- 脚本：`scripts/backup_brickplans.sh`。
- 文档：`README.md`、`docs/deployment.md`、`docs/production-initialization.md`、`docs/backup.md`、`docs/privacy-policy.md`。

发布包不应包含：

- `backend/brickplans.db`。
- `backend/uploads/`。
- `frontend/dist/`。
- `backend/.venv/`。
- `frontend/node_modules/`。
- `.env`、`.env.local`、`frontend/.env.production`。
- `backups/`、`runtime-archives/`。

## 4. 打包命令

```bash
cd /home/ubuntu/project/brickplans
mkdir -p releases

git archive \
  --format=tar.gz \
  --output=releases/brickplans-release-v1.0.0.tar.gz \
  HEAD

sha256sum releases/brickplans-release-v1.0.0.tar.gz > releases/brickplans-release-v1.0.0.tar.gz.sha256
```

如已创建 Git tag，则可把 `HEAD` 替换为 `v1.0.0`。

## 5. 生产初始化入口

部署到生产服务器后先执行：

```bash
less docs/production-initialization.md
```

不要直接复制当前开发环境的 SQLite 数据库和上传目录到生产。

## 6. 已知上线注意事项

- 生产必须覆盖默认 `SECRET_KEY`。
- 当前 SQLite 适合 Beta/小规模运营；并发和数据规模上来后再迁移 PostgreSQL。
- 前端构建后如浏览器不更新，先强刷或无痕窗口验证。
- 测试数据必须归档隔离，不能作为线上真实内容。
