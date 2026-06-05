# BrickPlans — 积木图纸分享社区

> 积木爱好者上传、浏览、分享 MOC 图纸的社区网站。

---

## 项目结构

```
brickplans/
├── backend/                   # FastAPI 后端
│   ├── app/
│   │   ├── api/               # 路由 / 接口
│   │   ├── core/              # 配置、依赖注入、安全
│   │   ├── models/            # SQLAlchemy 模型
│   │   ├── schemas/           # Pydantic 请求/响应模型
│   │   └── services/          # 业务逻辑层
│   ├── tests/                 # pytest 测试
│   └── alembic/               # 数据库迁移
├── frontend/                  # React + TypeScript + Tailwind
│   ├── src/
│   │   ├── components/        # 可复用组件
│   │   ├── pages/             # 页面组件
│   │   ├── hooks/             # 自定义 hooks
│   │   ├── lib/               # API 客户端、工具函数
│   │   └── types/             # TypeScript 类型定义
│   └── public/
├── docker/                    # Docker Compose + Dockerfiles
├── docs/                      # 文档、计划
└── design-sketches/           # UI 设计原型
    ├── playful-brick/         # 变体A: 乐高趣味风
    └── clean-gallery/         # 变体B: 简约画廊风
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端框架 | Python FastAPI |
| 前端 | React 18 + TypeScript + Tailwind CSS |
| 数据库 | PostgreSQL 15 |
| 缓存 | Redis |
| 文件存储 | MinIO (S3 兼容) |
| 认证 | JWT (access + refresh token) |
| 部署 | Docker Compose |

## 设计原型

在浏览器打开以下文件预览 UI：

```bash
# 变体 A — 乐高趣味风
open design-sketches/playful-brick/index.html

# 变体 B — 简约画廊风
open design-sketches/clean-gallery/index.html
```

三个页面（首页 / 发现 / 详情）均可点击导航切换，收藏按钮可交互。

## 核心功能

- 用户注册/登录
- 上传图纸（图片 + 零件清单 + 搭建说明）
- 按分类/难度/标签浏览筛选
- 收藏、评论
- 个人主页展示作品集
- 全文搜索
