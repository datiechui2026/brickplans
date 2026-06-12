# BrickPlans 部署手册

本文档记录当前单机部署方式：FastAPI 后端由 systemd 管理，Vite 前端构建产物由 nginx 静态托管，`/api/` 与 `/uploads/` 反向代理到后端。

## 端口与路径

| 项 | 值 |
|---|---|
| 项目目录 | `/home/ubuntu/project/brickplans` |
| 后端服务 | `brickplans-backend` |
| 后端监听 | `127.0.0.1:8100` |
| 前端监听 | `0.0.0.0:8310` |
| 前端静态目录 | `/home/ubuntu/project/brickplans/frontend/dist` |
| SQLite 数据库 | `/home/ubuntu/project/brickplans/backend/brickplans.db` |
| 上传目录 | `/home/ubuntu/project/brickplans/backend/uploads/` |

## 后端部署

首次部署：

```bash
cd /home/ubuntu/project/brickplans/backend
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
```

安装 systemd unit：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-backend.service /etc/systemd/system/brickplans-backend.service
sudo systemctl daemon-reload
sudo systemctl enable --now brickplans-backend
```

检查：

```bash
systemctl status brickplans-backend --no-pager
curl http://127.0.0.1:8100/api/health
```

更新后端代码后：

```bash
cd /home/ubuntu/project/brickplans/backend
. .venv/bin/activate
pip install -r requirements.txt
sudo systemctl restart brickplans-backend
curl http://127.0.0.1:8100/api/health
```

## 前端部署

构建：

```bash
cd /home/ubuntu/project/brickplans/frontend
npm ci
npm run build
```

安装 nginx 配置：

```bash
sudo cp /home/ubuntu/project/brickplans/brickplans-nginx.conf /etc/nginx/sites-available/brickplans
sudo ln -sf /etc/nginx/sites-available/brickplans /etc/nginx/sites-enabled/brickplans
sudo nginx -t
sudo systemctl reload nginx
```

检查：

```bash
curl http://127.0.0.1:8310/api/health
curl -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:8310/
```

## Vercel / Cloudflare Pages 前端部署

前端支持跨域 API base URL：

```bash
VITE_API_BASE_URL=http://YOUR_SERVER_HOST:8310
```

如果前后端同源部署在 nginx，保持 `VITE_API_BASE_URL` 为空，让请求走相对路径 `/api`。

## 回滚

代码回滚：

```bash
git log --oneline -5
git checkout <commit>
cd frontend && npm ci && npm run build
sudo systemctl restart brickplans-backend
sudo systemctl reload nginx
```

数据回滚见 `docs/backup.md`。

## 常见问题

- 前端看不到更新：先 `Ctrl+Shift+R` 强刷；确认 `frontend/dist/assets/index-*.js` 的时间戳是否已更新。
- `/uploads/` 404：确认 nginx 配置里 `location ^~ /uploads/` 代理到 `127.0.0.1:8100`，不要被静态资源 regex location 抢走。
- 后端测试找不到 `sqlalchemy`：使用 `backend/.venv/bin/python -m pytest`，不要用系统 Python。
- 仓库里的 `frontend/dist/` 不提交；部署机器本地构建即可。
