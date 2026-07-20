# BrickPlan — 积木图纸分享社区

BrickPlan 是一个面向积木/MOC 爱好者的图纸分享社区。当前版本已经完成可运行 MVP：用户可以注册登录、上传图纸、管理图片和标签、浏览搜索、点赞收藏、评论举报，管理员可以做基础内容审核。

## 当前技术栈

| 层 | 实际实现 |
|---|---|
| 后端 | FastAPI + SQLAlchemy Async + Pydantic v2 |
| 数据库 | SQLite（本机部署），代码保留 PostgreSQL 依赖用于后续迁移 |
| 前端 | Vite + Vanilla JavaScript + 单文件 CSS 设计系统 |
| 认证 | JWT access token + refresh token |
| 文件 | 本地 `backend/uploads/`，由 FastAPI 静态挂载并经 nginx 代理 |
| 部署 | systemd 后端服务 + nginx 静态前端/API 反代 |
| 测试 | pytest + pytest-asyncio，前端用 Vite build 校验 |

> 注意：早期规划里的 React/Tailwind/PostgreSQL/Redis/MinIO 不是当前落地形态。

## 项目结构

```text
brickplans/
├── backend/
│   ├── app/
│   │   ├── api/              # auth, blueprints, images, tags, users, admin 等路由
│   │   ├── core/             # 配置、数据库、安全
│   │   ├── models/           # SQLAlchemy models
│   │   ├── schemas/          # Pydantic schemas
│   │   └── services/         # 文件存储等服务
│   ├── tests/                # pytest 回归测试
│   ├── seed.py               # 本地种子数据脚本
│   └── requirements.txt
├── frontend/
│   ├── src/
│   │   ├── main.js           # SPA 页面渲染和交互
│   │   ├── api.js            # API client + token refresh
│   │   └── styles/main.css   # 设计系统和页面样式
│   ├── public/avatars/       # 预设头像
│   └── package.json
├── docs/                     # PRD、部署、备份、隐私政策
├── sketches/                 # 页面设计稿
├── brickplans-backend.service
└── brickplans-nginx.conf
```

## 已完成功能

- 用户：注册、登录、自动刷新 token、个人资料、改密码、头像上传/预设头像。
- 图纸：创建、编辑、删除、详情、列表、搜索、分类、标签筛选、浏览数。
- 图片：多图上传、删除、排序、设置封面、详情页轮播与灯箱预览。
- 互动：收藏、点赞、评论、举报入口。
- 内容管理：管理员图纸列表、待审核列表、发布/下架/删除。
- 公开页面：首页、发现页、详情页、上传页、编辑页、个人主页、后台页、隐私页、错误页。
- SEO/运营：`robots.txt`、sitemap 接口、统计接口、PRD 文档。

## 本地开发

### 后端

```bash
cd backend
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --reload --host 127.0.0.1 --port 8100
```

健康检查：

```bash
curl http://127.0.0.1:8100/api/health
```

### 前端

```bash
cd frontend
npm install
npm run dev
```

前端 API 默认走同源 `/api`。本机 Vite 开发如需直连后端，可以创建未提交的 `.env.local`：

```bash
VITE_API_BASE_URL=http://127.0.0.1:8100
```

## 测试与构建

推荐提交前执行：

```bash
# 前端语法检查
node --check frontend/src/main.js

# 后端测试：使用 systemd 同款 venv
backend/.venv/bin/python -m pytest -q backend/tests

# 前端生产构建
cd frontend && npm run build
```

当前基线：后端 `60 passed`，前端 Vite build 通过。

## 部署概览

本机部署约定：

- 后端 systemd：`brickplans-backend`，监听 `127.0.0.1:8100`
- 前端 nginx：监听 `0.0.0.0:8310`
- nginx 静态目录：`frontend/dist`
- nginx `/api/` 和 `/uploads/` 代理到后端 `127.0.0.1:8100`

部署验证：

```bash
systemctl is-active brickplans-backend
curl http://127.0.0.1:8100/api/health
curl http://127.0.0.1:8310/api/health
curl -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:8310/
```

更多细节见：

- `docs/deployment.md`：生产部署手册。
- `docs/production-initialization.md`：上线初始化、测试数据清理、管理员创建。
- `docs/release-v1.0.0.md`：v1.0.0 发布清单和打包说明。

## 数据与备份

运行态数据不进 git：

- SQLite：`backend/brickplans.db`
- 上传文件：`backend/uploads/`
- 环境文件：`.env*`、`frontend/.env.production`
- 构建产物：`frontend/dist/`

备份方案见 `docs/backup.md`，脚本见 `scripts/backup_brickplans.sh`。

## 下一步产品方向

建议下一迭代聚焦社区互动闭环：站内通知 + 评论回复。详见 `docs/PRD-27-notifications-comment-replies.md`。
