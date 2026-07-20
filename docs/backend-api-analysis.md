# BrickPlans 后端接口分析 & 模拟数据导入方案

> 分析日期：2026-06-27 | 分析人：开发02AI

---

## 1. 当前环境概览

| 项目 | 值 |
|------|-----|
| 数据库 | SQLite (`backend/brickplans.db`) |
| 存储后端 | 腾讯 COS (`brickplan2-1317217681`, ap-hongkong) |
| 后端服务 | systemd `brickplans-backend` (active) |
| 后端端口 | `127.0.0.1:8100` |
| 前端端口 | `127.0.0.1:8310` (nginx → 8100) |
| JWT Secret | `bp-production-key-change-me-please-2026` |
| JWT 算法 | HS256 |
| Access Token 有效期 | 60 分钟 |
| Refresh Token 有效期 | 30 天 |
| 当前用户数 | 20 |
| 当前图纸数 | 49 |
| 当前图片数 | 67 |

---

## 2. 数据模型

### 2.1 ER 关系

```
User (用户)
 ├──< Blueprint (图纸)
 │      ├──< BlueprintImage (图片/PDF)
 │      ├──< BlueprintTag >── Tag (标签)
 │      ├──< Comment (评论，支持一级回复)
 │      ├──< Favorite (收藏)
 │      ├──< Like (点赞)
 │      └──< Report (举报)
 └──< Notification (通知)
```

### 2.2 User 表

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID string | 主键，自动生成 |
| `username` | String(30) | 唯一，索引 |
| `email` | String(255) | 唯一 |
| `password_hash` | String(255) | bcrypt 加密 |
| `avatar_url` | String(500) | 可选，默认随机 preset |
| `bio` | Text | 可选 |
| `is_admin` | Boolean | 默认 False |
| `created_at` | DateTime | UTC |

### 2.3 Blueprint 表

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID string | 主键，自动生成 |
| `author_id` | UUID FK → users.id | 作者 |
| `title` | String(100) | 必填 |
| `slug` | String(120) | 自动从 title 生成，索引 |
| `description` | Text | 可选 |
| `difficulty` | Integer | 可选，1-5 |
| `piece_count` | Integer | 可选，零件数 |
| `category` | String(30) | 可选，索引 |
| `dimensions` | String(50) | 可选 |
| `part_list` | JSON | 可选，零件清单 |
| `view_count` | Integer | 默认 0 |
| `like_count` | Integer | 默认 0 |
| `is_published` | Boolean | 默认 True，False=仅作者可见 |
| `created_at` | DateTime | UTC |
| `updated_at` | DateTime | UTC，自动更新 |

### 2.4 BlueprintImage 表

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID string | 主键 |
| `blueprint_id` | UUID FK → blueprints.id | 所属图纸 |
| `url` | String(500) | 访问 URL（COS 或 /uploads/） |
| `object_key` | String(500) | COS key，索引 |
| `sort_order` | Integer | 排序 |
| `is_cover` | Boolean | 是否为封面 |
| `file_type` | String(10) | `"image"` 或 `"pdf"` |

---

## 3. API 路由全景

### 3.1 认证模块 (`/api/auth`)

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/auth/register` | 无 | 注册 → 返回 TokenResponse (access_token + refresh_token + user) |
| POST | `/api/auth/login` | 无 | 登录 → 返回 TokenResponse |
| POST | `/api/auth/refresh` | 无 | 刷新 token |
| GET | `/api/auth/me` | Bearer | 获取当前用户信息 |
| PUT | `/api/auth/me` | Bearer | 更新用户名/bio/头像 |
| PUT | `/api/auth/password` | Bearer | 修改密码 |
| POST | `/api/auth/avatar` | Bearer | 上传头像 (≤2MB, jpg/png/webp/gif) |
| GET | `/api/auth/avatars` | 无 | 获取预设头像列表 (20个) |

### 3.2 图纸模块 (`/api/blueprints`)

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/blueprints` | Bearer | **创建图纸** |
| GET | `/api/blueprints` | 可选 | 列表（分页/搜索/分类/标签/排序） |
| GET | `/api/blueprints/{id}` | 可选 | 图纸详情（未发布图纸仅作者可见） |
| PUT | `/api/blueprints/{id}` | Bearer | 更新图纸（仅作者） |
| DELETE | `/api/blueprints/{id}` | Bearer | 删除图纸（仅作者） |

#### 列表查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | 1 | 页码 |
| `size` | int | 12 | 每页数量 (max 50) |
| `q` | string | - | 搜索关键词（标题+描述） |
| `category` | string | - | 分类筛选 |
| `tag` | string | - | 标签筛选 |
| `sort` | string | `new` | `new` 或 `popular` |

### 3.3 图片/文件模块 (`/api/blueprints/{id}/images`)

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/blueprints/{id}/images` | Bearer | **上传文件**（多文件，仅作者） |
| DELETE | `/api/blueprints/{id}/images/{img_id}` | Bearer | 删除文件（仅作者） |
| PUT | `/api/blueprints/{id}/images/{img_id}/cover` | Bearer | 设为封面（仅作者） |
| PUT | `/api/blueprints/{id}/images/reorder` | Bearer | 重排文件（仅作者） |

**上传限制：**
- 支持格式：JPG / PNG / WebP / PDF
- 最大 20MB
- 图片自动压缩（resize ≤2048px, JPEG quality 80）
- PDF 自动压缩（pypdf compress_content_streams）
- 存储到腾讯 COS，ACL 设为 public-read

### 3.4 互动模块

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/blueprints/{id}/favorite` | Bearer | 收藏 |
| DELETE | `/api/blueprints/{id}/favorite` | Bearer | 取消收藏 |
| POST | `/api/blueprints/{id}/like` | Bearer | 点赞 |
| DELETE | `/api/blueprints/{id}/like` | Bearer | 取消点赞 |
| GET | `/api/blueprints/{id}/comments` | 无 | 评论列表 |
| POST | `/api/blueprints/{id}/comments` | Bearer | 发表评论（支持 parent_id 回复） |

### 3.5 其他模块

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/users/{id}` | 用户主页 |
| GET | `/api/users/{id}/blueprints` | 用户图纸列表 |
| GET | `/api/users/{id}/favorites` | 用户收藏列表 |
| GET | `/api/tags` | 标签列表 |
| GET | `/api/stats` | 站点统计 |
| GET | `/api/seo/sitemap.xml` | SEO 站点地图 |
| GET | `/api/health` | 健康检查 |

---

## 4. 关键 Schema

### 4.1 注册请求 `UserRegister`

```json
{
  "username": "string (2-30 chars)",
  "email": "email",
  "password": "string (6-128 chars)"
}
```

### 4.2 注册/登录响应 `TokenResponse`

```json
{
  "access_token": "jwt...",
  "refresh_token": "jwt...",
  "token_type": "bearer",
  "user": {
    "id": "uuid",
    "username": "string",
    "email": "email",
    "avatar_url": "/avatars/presets/03.png",
    "bio": null,
    "is_admin": false,
    "created_at": "2026-06-27T..."
  }
}
```

### 4.3 创建图纸请求 `BlueprintCreate`

```json
{
  "title": "string (1-100 chars, 必填)",
  "description": "string (可选)",
  "difficulty": 3,          // 可选, 1-5
  "piece_count": 1200,      // 可选
  "category": "建筑",        // 可选
  "dimensions": "30x20x15", // 可选
  "part_list": {},          // 可选, JSON
  "is_published": true      // 默认 true
}
```

### 4.4 上传图片响应

```json
[
  {
    "id": "uuid",
    "blueprint_id": "uuid",
    "url": "https://brickplan2-1317217681.cos.ap-hongkong.myqcloud.com/blueprints/abc123.jpg",
    "object_key": "blueprints/abc123.jpg",
    "sort_order": 0,
    "is_cover": false,
    "file_type": "image"
  }
]
```

---

## 5. 认证流程

```
注册/登录 → 获取 access_token + refresh_token
                ↓
         所有需认证请求带 Header:
         Authorization: Bearer {access_token}
                ↓
         deps.py: get_current_user()
           1. 解码 JWT (HS256)
           2. 验证 type == "access"
           3. 用 sub (user_id) 查库
           4. 返回 User 对象
```

**JWT Payload 结构：**
```json
{
  "sub": "user-uuid-string",
  "exp": 1234567890,
  "type": "access"
}
```

---

## 6. 存储架构

```
请求 → FastAPI (8100)
         │
         ├── 本地存储: /backend/uploads/{prefix}/{uuid}.{ext}
         │     URL: /uploads/{prefix}/{uuid}.{ext}
         │
         └── 腾讯 COS: brickplan2-1317217681.cos.ap-hongkong.myqcloud.com
               URL: https://brickplan2-1317217681.cos.ap-hongkong.myqcloud.com/{prefix}/{uuid}.{ext}
               ACL: public-read
               PDF: ContentDisposition=inline
```

当前环境使用 **腾讯 COS**（`.env` 中 `STORAGE_BACKEND=tencent_cos`）。

---

## 7. 模拟导入方案

### 7.1 整体流程

```
① 创建用户 → POST /api/auth/register
② 创建图纸 → POST /api/blueprints
③ 上传图片/PDF → POST /api/blueprints/{id}/images
④ (可选) 添加标签 → 需要额外 API 或直接写 DB
```

### 7.2 脚本设计要点

#### 用户创建
- 直接调 `/api/auth/register`，无需验证码
- 返回的 `access_token` 用于后续图纸创建和图片上传
- 用户名/邮箱需要唯一，建议用模式 `sim_user_{i}@brickplan.cn`

#### 图纸创建
- `title` 必填，`slug` 自动从 title 生成
- `is_published=True` → 公开可见；`False` → 仅作者可见
- 返回的 `id` 用于后续图片上传

#### 图片上传
- 使用 `multipart/form-data`，字段名 `files`
- 支持批量上传（一次传多个文件）
- 图片自动压缩，PDF 自动压缩
- 存储到 COS，返回公开 URL

#### 标签
- 当前 API 没有独立的"给图纸加标签"端点
- 标签在创建/更新图纸时通过 `tags` 字段传入
- 但 `BlueprintCreate` schema 中没有 `tags` 字段
- **方案：** 创建图纸后，直接写 SQLite 插入 `tags` 和 `blueprint_tags` 表

### 7.3 数据来源适配

根据目标平台不同，数据获取方式：

| 平台 | 方式 | 复杂度 |
|------|------|--------|
| Rebrickable | REST API (需 API key) | 低 |
| BrickLink | 爬虫 | 中 |
| LEGO 官网 | 爬虫 | 中 |
| 国内积木社区 | 爬虫 | 中-高 |

### 7.4 待确认事项

| # | 问题 | 影响 |
|---|------|------|
| 1 | **目标平台？** | 决定数据获取方式（API vs 爬虫） |
| 2 | **用户数量？** | 决定注册循环规模 |
| 3 | **每用户图纸数？** | 决定数据量 |
| 4 | **是否下载图片/PDF？** | 决定是否需要文件下载+上传逻辑 |
| 5 | **图纸公开还是草稿？** | 决定 `is_published` 值 |

---

## 8. 已知坑点（来自 skill）

1. **`create_access_token` bug** — `sub` 嵌套为 dict 而非 string，但 register 接口调它后 `get_current_user` 能正常工作（取 `claims.get("sub")` 当 user_id 查库）
2. **未发布图纸 → 404** — 这是权限检查，不是路由 bug。`is_published=False` 的图纸只有作者能看到
3. **`cover_url` 计算跳过 PDF** — 封面图不会选 PDF 文件
4. **`file_type` 必须序列化** — 所有 image 响应路径都要包含 `file_type`，否则前端无法区分图片和 PDF
5. **Pydantic `response_model` 静默丢弃未声明字段** — 返回 dict 中多余的字段会被 FastAPI 丢弃

---

## 9. 快速验证命令

```bash
# 健康检查
curl -s http://127.0.0.1:8100/api/health

# 注册用户
curl -s -X POST http://127.0.0.1:8100/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"test01","email":"test01@test.com","password":"123456"}'

# 登录
curl -s -X POST http://127.0.0.1:8100/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test01@test.com","password":"123456"}'

# 创建图纸 (替换 TOKEN)
curl -s -X POST http://127.0.0.1:8100/api/blueprints \
  -H 'Authorization: Bearer TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{"title":"测试图纸","description":"描述","difficulty":3,"piece_count":500,"category":"建筑","is_published":true}'

# 上传图片 (替换 TOKEN 和 BLUEPRINT_ID)
curl -s -X POST http://127.0.0.1:8100/api/blueprints/BLUEPRINT_ID/images \
  -H 'Authorization: Bearer TOKEN' \
  -F 'files=@/path/to/image.jpg'
```
