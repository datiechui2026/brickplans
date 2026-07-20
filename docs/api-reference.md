# BrickPlan API 接口文档

> 自动生成于 2026-06-14 | 后端版本 0.1.0

---

## 1. 认证 `/api/auth`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| POST | `/api/auth/register` | 无 | 注册 | `UserRegister` { username, email, password } | `TokenResponse` { access_token, refresh_token, user } |
| POST | `/api/auth/login` | 无 | 登录 | `UserLogin` { email, password } | `TokenResponse` { access_token, refresh_token, user } |
| POST | `/api/auth/refresh` | 无 | 刷新 token | `RefreshRequest` { refresh_token } | `TokenResponse` { access_token, refresh_token } |
| GET | `/api/auth/me` | Bearer | 获取当前用户 | — | `UserOut` { id, username, email, avatar_url, bio, is_admin, created_at } |
| PUT | `/api/auth/me` | Bearer | 更新个人信息 | `UserUpdateRequest` { username?, bio?, avatar_url? } | `UserOut` |
| PUT | `/api/auth/password` | Bearer | 修改密码 | `PasswordChangeRequest` { current_password, new_password } | `{ message }` |
| POST | `/api/auth/avatar` | Bearer | 上传头像 | `multipart file` (≤2MB, jpg/png/webp/gif) | `{ avatar_url, object_key, user }` |
| GET | `/api/auth/avatars` | 无 | 预设头像列表 | — | `{ avatars: [{ id, url }] }` (20个) |

---

## 2. 蓝图 `/api/blueprints`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| POST | `/api/blueprints` | Bearer | 创建蓝图 | `BlueprintCreate` { title*, description?, difficulty?(1-5), piece_count?, category?, dimensions?, part_list?, is_published? } | `BlueprintOut` |
| GET | `/api/blueprints` | 可选 | 蓝图列表 | Query: `page`(1), `size`(12), `q`(搜索), `category`, `sort`(new/popular), `tag` | `BlueprintListOut` { items[], total, page, page_size } |
| GET | `/api/blueprints/{id}` | 可选 | 蓝图详情 | Path: id | `BlueprintDetail` (含 is_favorited, favorite_count) |
| PUT | `/api/blueprints/{id}` | Bearer | 更新蓝图 | `BlueprintUpdate` (所有字段可选) | `BlueprintOut` |
| DELETE | `/api/blueprints/{id}` | Bearer | 删除蓝图 | Path: id | 204 No Content |
| POST | `/api/blueprints/{id}/favorite` | Bearer | 收藏 | Path: id | `{ detail: "Favorited" }` |
| DELETE | `/api/blueprints/{id}/favorite` | Bearer | 取消收藏 | Path: id | 204 No Content |
| POST | `/api/blueprints/{id}/like` | Bearer | 点赞 | Path: id | `{ detail, like_count }` |
| DELETE | `/api/blueprints/{id}/like` | Bearer | 取消点赞 | Path: id | 204 No Content |
| POST | `/api/blueprints/{id}/comments` | Bearer | 发表评论 | `CommentCreate` { content*, parent_id? } | `CommentOut` |
| GET | `/api/blueprints/{id}/comments` | 无 | 评论列表 | Path: id | `CommentOut[]` |

### BlueprintOut 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | str | UUID |
| author_id | str | 作者 ID |
| title | str | 标题 |
| slug | str | URL 友好标识 |
| description | str\|null | 描述 |
| difficulty | int\|null | 难度 1-5 |
| piece_count | int\|null | 零件数 |
| category | str\|null | 分类 |
| dimensions | str\|null | 尺寸 |
| part_list | dict\|null | 零件清单 |
| view_count | int | 浏览数 |
| like_count | int | 点赞数 |
| favorite_count | int | 收藏数 |
| is_liked | bool | 当前用户是否点赞 |
| cover_url | str\|null | 封面图 URL |
| is_published | bool | 是否发布 |
| created_at | str | ISO 时间 |
| updated_at | str | ISO 时间 |
| author | UserOut\|null | 作者信息 |
| images | BlueprintImageOut[] | 图片列表 |
| tags | str[] | 标签名列表 |
| moderation_status | str\|null | "审核中" 或 null |

### BlueprintDetail 额外字段

| 字段 | 类型 | 说明 |
|------|------|------|
| is_favorited | bool | 当前用户是否收藏 |

---

## 3. 图片管理 `/api/blueprints/{id}/images`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| POST | `/api/blueprints/{id}/images` | Bearer(作者) | 上传图片 | `multipart files[]` (≤10MB, jpg/png/webp) | `BlueprintImageOut[]` |
| PUT | `/api/blueprints/{id}/images/{img_id}/cover` | Bearer(作者) | 设置封面 | Path: id, img_id | `{ message: "ok" }` |
| PUT | `/api/blueprints/{id}/images/reorder` | Bearer(作者) | 重排图片 | `{ images: [{ id, sort_order }] }` | `{ message: "ok" }` |
| DELETE | `/api/blueprints/{id}/images/{img_id}` | Bearer(作者) | 删除图片 | Path: id, img_id | 204 No Content |

### BlueprintImageOut

| 字段 | 类型 | 说明 |
|------|------|------|
| id | str | UUID |
| url | str | 图片 URL |
| object_key | str\|null | 存储 key |
| sort_order | int | 排序 |
| is_cover | bool | 是否封面 |

---

## 4. 标签 `/api`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| GET | `/api/tags` | 无 | 全部标签 | — | `[{ id, name }]` |
| GET | `/api/blueprints/{id}/tags` | 无 | 蓝图标签 | Path: id | `[{ id, name }]` |
| POST | `/api/blueprints/{id}/tags` | Bearer(作者) | 批量打标签 | `{ tags: string[] }` (1-20个) | `[{ id, name }]` |
| DELETE | `/api/blueprints/{id}/tags/{tag_id}` | Bearer(作者) | 移除标签 | Path: id, tag_id | 204 No Content |

---

## 5. 用户 `/api/users`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| GET | `/api/users/{user_id}` | 无 | 用户主页 | Path: user_id (支持 id 或 username 回退) | `UserOut` + `blueprint_count`, `favorite_count` |
| GET | `/api/users/{user_id}/blueprints` | 无 | 用户作品 | Query: `page`, `size` | `BlueprintListOut` |
| GET | `/api/users/{user_id}/favorites` | 无 | 用户收藏 | Query: `page`, `size` | `BlueprintListOut` (仅已发布) |

---

## 6. 举报 `/api/reports`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| POST | `/api/reports` | Bearer | 举报蓝图 | `ReportCreate` { blueprint_id*, reason*(inappropriate/copyright/spam/other), detail? } | `ReportOut` |

> 同一用户对同一蓝图只能举报一次。累计 ≥3 次举报自动下架。

### ReportOut

| 字段 | 类型 | 说明 |
|------|------|------|
| id | str | UUID |
| reporter_id | str | 举报人 ID |
| blueprint_id | str | 被举报蓝图 ID |
| reason | str | 举报原因 |
| detail | str\|null | 详细说明 |
| status | str | 状态 (pending) |
| created_at | str | ISO 时间 |

---

## 7. 通知 `/api/notifications`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| GET | `/api/notifications` | Bearer | 通知列表 | Query: `page`, `size` | `NotificationListOut` { items[], total, unread_count, page, page_size } |
| GET | `/api/notifications/unread-count` | Bearer | 未读数 | — | `{ unread_count }` |
| POST | `/api/notifications/mark-read` | Bearer | 全部已读 | — | `{ detail }` |

### NotificationOut

| 字段 | 类型 | 说明 |
|------|------|------|
| id | str | UUID |
| user_id | str | 接收者 ID |
| actor_id | str\|null | 触发者 ID |
| type | str | 类型: comment / comment_reply / like / favorite |
| blueprint_id | str\|null | 关联蓝图 |
| comment_id | str\|null | 关联评论 |
| payload | dict\|null | 附加数据 (blueprint_title, comment_excerpt) |
| is_read | bool | 是否已读 |
| created_at | str | ISO 时间 |
| read_at | str\|null | 已读时间 |
| actor | UserOut\|null | 触发者信息 |

---

## 8. 统计 `/api`

| 方法 | 路径 | 认证 | 说明 | 响应 |
|------|------|------|------|------|
| GET | `/api/stats` | 无 | 平台统计 | `StatsResponse` { total_blueprints, total_users, total_favorites, total_pieces, total_views, total_likes } |

---

## 9. SEO

| 方法 | 路径 | 认证 | 说明 | 响应 |
|------|------|------|------|------|
| GET | `/sitemap.xml` | 无 | XML 站点地图 | `application/xml` (已发布蓝图的 URL 列表) |

---

## 10. 管理员 `/api/admin`

| 方法 | 路径 | 认证 | 说明 | 请求体/参数 | 响应 |
|------|------|------|------|------------|------|
| GET | `/api/admin/blueprints` | Bearer(admin) | 全部作品 | Query: `page`, `size`, `q`(标题/作者搜索) | `BlueprintListOut` |
| GET | `/api/admin/blueprints/pending` | Bearer(admin) | 待审核列表 | Query: `page`, `size` | `BlueprintListOut` (is_published=false) |
| PUT | `/api/admin/blueprints/{id}/publish` | Bearer(admin) | 审核通过 | Path: id | `{ detail: "Published" }` |
| PUT | `/api/admin/blueprints/{id}/unpublish` | Bearer(admin) | 下架 | Path: id | `{ detail: "Unpublished" }` |
| DELETE | `/api/admin/blueprints/{id}` | Bearer(admin) | 删除作品 | Path: id | 204 No Content |

---

## 11. 健康检查

| 方法 | 路径 | 认证 | 说明 | 响应 |
|------|------|------|------|------|
| GET | `/api/health` | 无 | 健康检查 | `{ status: "ok", version: "0.1.0" }` |

---

## 通用说明

### 认证方式

```
Authorization: Bearer <access_token>
```

- `access_token` 和 `refresh_token` 均为 JWT
- 标注 `可选` 的接口：传 token 则返回用户相关状态（is_liked, is_favorited），不传则返回默认值

### 分页

统一格式：

```
Query: page (默认 1), size (默认 12/20)
Response: { items[], total, page, page_size }
```

### 错误响应

| 状态码 | 含义 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未认证 / token 无效 |
| 403 | 无权限（非作者/非管理员） |
| 404 | 资源不存在 |
| 409 | 冲突（重复注册/重复收藏等） |
| 413 | 文件过大 |
| 422 | 参数校验失败 |

### 接口统计

| 模块 | 接口数 |
|------|--------|
| Auth | 8 |
| Blueprints | 11 |
| Images | 4 |
| Tags | 4 |
| Users | 3 |
| Reports | 1 |
| Notifications | 3 |
| Stats | 1 |
| SEO | 1 |
| Admin | 5 |
| Health | 1 |
| **合计** | **42** |
