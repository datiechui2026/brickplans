# BrickPlan 上传作品 API 说明

> 面向注册用户。所有写操作需携带 JWT Token。

---

## API 基础信息

| 项目 | 值 |
|------|-----|
| 基础路径 | `http://124.221.85.247:8310/api` |
| 认证方式 | Header: `Authorization: Bearer <access_token>` |
| Content-Type | `application/json`（图片接口用 `multipart/form-data`） |

---

## 上传流程（3 步）

### 步骤 1：注册 / 登录

#### 注册 `POST /api/auth/register`

```json
// Request Body
{
  "username": "积木大师Jack",
  "email": "jack@example.com",
  "password": "mypass123"
}

// Response 201
{
  "access_token": "eyJhbG...",
  "refresh_token": "eyJhbG...",
  "token_type": "bearer",
  "user": {
    "id": "uuid",
    "username": "积木大师Jack",
    "email": "jack@example.com",
    "avatar_url": null,
    "bio": null,
    "is_admin": false,
    "created_at": "2026-06-07T..."
  }
}
```

| 字段 | 类型 | 必填 | 约束 |
|------|------|------|------|
| username | string | ✅ | 2-30 字符 |
| email | string | ✅ | 合法邮箱 |
| password | string | ✅ | 6-128 字符 |

#### 登录 `POST /api/auth/login`

```json
// Request
{ "email": "jack@example.com", "password": "mypass123" }

// Response 200 — 格式同上
```

> ⚠️ access_token 有效期 30 分钟，过期后用 refresh_token 调用 `POST /api/auth/refresh` 续期。

---

### 步骤 2：创建图纸 `POST /api/blueprints`

```json
// Request
{
  "title": "经典太空飞船",
  "description": "一款经典的科幻积木设计...",
  "difficulty": 3,
  "piece_count": 450,
  "category": "科幻",
  "dimensions": "30x20x15 cm",
  "part_list": {
    "parts": [
      { "name": "2x4 基础砖", "count": 120, "color": "白色" },
      { "name": "1x2 斜面砖", "count": 45, "color": "灰色" }
    ],
    "total": 450
  },
  "is_published": true
}

// Response 201
{
  "id": "bp_uuid",
  "author_id": "user_uuid",
  "title": "经典太空飞船",
  "slug": "经典太空飞船",
  "description": "一款经典的科幻积木设计...",
  "difficulty": 3,
  "piece_count": 450,
  "category": "科幻",
  "dimensions": "30x20x15 cm",
  "part_list": { "parts": [...], "total": 450 },
  "view_count": 0,
  "like_count": 0,
  "favorite_count": 0,
  "is_liked": false,
  "cover_url": null,
  "is_published": true,
  "created_at": "2026-06-07T14:30:00",
  "updated_at": "2026-06-07T14:30:00",
  "author": { "id": "uuid", "username": "积木大师Jack", ... },
  "images": [],
  "tags": []
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | ✅ | 1-100 字符 |
| description | string | ❌ | 作品描述 |
| difficulty | int | ❌ | 1-5 难度等级 |
| piece_count | int | ❌ | 零件数 |
| category | string | ❌ | 建筑/车辆/机甲/奇幻/科幻/场景 |
| dimensions | string | ❌ | 尺寸文本 |
| part_list | object | ❌ | `{ parts: [{name, count, color}], total }` |
| is_published | bool | ❌ | 默认 true。false = 草稿，仅自己可见 |

---

### 步骤 3：上传图片 `POST /api/blueprints/{blueprint_id}/images`

```
Content-Type: multipart/form-data

字段: files（可批量，多张图）
```

| 参数 | 说明 |
|------|------|
| files | File 数组，支持 JPG / PNG / WebP |
| 大小限制 | 单张 ≤ 10MB |
| 权限 | 只能上传自己作品的图片 |

**Response 201** — 返回该图纸所有图片列表：
```json
[
  { "id": "img_uuid_1", "url": "/uploads/xxx.jpg", "sort_order": 0, "is_cover": false },
  { "id": "img_uuid_2", "url": "/uploads/yyy.png", "sort_order": 1, "is_cover": false }
]
```

#### 辅助操作

| 操作 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 设置封面 | PUT | `/{bp_id}/images/{img_id}/cover` | 将指定图片设为封面 |
| 重排序 | PUT | `/{bp_id}/images/reorder` | Body: `{ images: [{id, sort_order}] }` |
| 删除图片 | DELETE | `/{bp_id}/images/{img_id}` | 永久删除 |
| 编辑标题等 | PUT | `/{bp_id}` | Body 同创建但字段均为可选 |
| 删除作品 | DELETE | `/{bp_id}` | 永久删除 |

---

## 完整上传流程示例（curl）

```bash
# 1. 登录获取 token
TOKEN=$(curl -s -X POST http://124.221.85.247:8310/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"jack@example.com","password":"mypass123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# 2. 创建图纸
BP_ID=$(curl -s -X POST http://124.221.85.247:8310/api/blueprints \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"我的作品","difficulty":3,"piece_count":250,"category":"建筑"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

# 3. 上传图片
curl -X POST http://124.221.85.247:8310/api/blueprints/$BP_ID/images \
  -H "Authorization: Bearer $TOKEN" \
  -F "files=@photo1.jpg" \
  -F "files=@photo2.png"
```

---

## 错误码速查

| HTTP | 含义 |
|------|------|
| 201 | 创建成功 |
| 401 | Token 过期或无效 → 刷新或重新登录 |
| 403 | 无权限（不是作者） |
| 404 | 图纸不存在 |
| 413 | 图片超过 10MB |
| 422 | 参数校验失败（格式/必填不满足） |
