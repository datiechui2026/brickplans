# PRD-27 — 站内通知 + 评论回复

## 背景

BrickPlan 当前已经具备上传、浏览、收藏、点赞、评论和举报能力，但互动没有闭环：

- 作者不知道自己的图纸被评论、点赞或收藏。
- 用户评论后无法针对某条评论回复。
- 用户回访时没有“有新互动”的提示。

下一迭代建议优先补齐站内通知和评论回复，提高社区活跃和回访动机。

## 目标

1. 用户能回复某条评论，形成一层回复关系。
2. 图纸作者收到新评论、评论回复、点赞、收藏通知。
3. 用户能在导航栏看到未读通知红点/数量。
4. 用户能进入通知页查看、标记已读、跳转到相关图纸。

## 非目标

- 不做实时 WebSocket。
- 不做多级嵌套评论，MVP 只支持一级回复。
- 不做邮件/短信/飞书外部通知。
- 不做复杂消息模板配置后台。

## 用户故事

### 作者收到评论通知

作为图纸作者，当其他用户评论我的图纸时，我希望收到通知，并能点开回到图纸详情。

验收：

- 评论自己的图纸不产生通知。
- 同一条评论产生一条通知。
- 通知内容包含评论者、图纸标题、评论摘要。

### 用户收到回复通知

作为评论者，当别人回复我的评论时，我希望收到通知。

验收：

- 回复自己的评论不产生通知。
- 回复通知跳转到对应图纸详情。
- 通知内容包含回复者和回复摘要。

### 导航栏未读红点

作为登录用户，当有未读通知时，我希望在导航栏看到数量提示。

验收：

- 未登录用户不显示通知入口。
- 未读数为 0 时不显示红点。
- 未读数 > 99 时显示 `99+`。

### 通知列表

作为登录用户，我希望进入通知页查看互动历史。

验收：

- 默认按创建时间倒序。
- 支持单条标记已读。
- 支持一键全部已读。
- 点击通知跳转相关图纸。

## 数据模型建议

### comments 增量字段

```text
parent_id: UUID nullable -> comments.id
```

约束：

- `parent_id` 为空：顶层评论。
- `parent_id` 不为空：一级回复。
- 回复不允许再被回复成多级嵌套；如果目标评论已有 `parent_id`，后端可把回复挂到其顶层父评论，或返回 400。建议 MVP 返回 400，逻辑更清晰。

### notifications 新表

```text
id: UUID primary key
user_id: UUID -> users.id
actor_id: UUID nullable -> users.id
type: string          # comment | reply | like | favorite | admin
blueprint_id: UUID nullable -> blueprints.id
comment_id: UUID nullable -> comments.id
payload: JSON nullable
is_read: bool default false
created_at: datetime
read_at: datetime nullable
```

索引：

- `(user_id, is_read, created_at)`
- `(user_id, created_at)`

## API 设计

### 评论回复

```http
POST /api/blueprints/{blueprint_id}/comments
{
  "content": "回复内容",
  "parent_id": "comment_uuid | null"
}
```

兼容现有评论接口：`parent_id` 可选。

评论列表返回：

```json
{
  "id": "...",
  "content": "...",
  "parent_id": null,
  "reply_count": 2,
  "replies": []
}
```

MVP 可返回扁平列表，由前端按 `parent_id` 分组渲染。

### 通知

```http
GET /api/notifications?status=unread&page=1&size=20
GET /api/notifications/unread-count
PUT /api/notifications/{id}/read
PUT /api/notifications/read-all
```

权限：只能访问自己的通知。

## 前端页面

### 导航栏

- 登录态显示 `🔔` 按钮。
- 调用 `getUnreadNotificationCount()`。
- 首次加载和登录后拉取一次；后续每 60 秒轮询一次即可。

### 通知页 `#/notifications`

布局：

```text
.main
  .form-card / .notifications-card
    h1 通知
    button 全部已读
    notification-list
      notification-item unread/read
```

通知 item 文案示例：

- `小明 评论了你的图纸「迷你城堡」：这个配色很好看`
- `小红 回复了你的评论：我也用了这个结构`
- `小张 点赞了你的图纸「太空车」`
- `小李 收藏了你的图纸「花店」`

### 详情页评论区

- 每条评论下方显示 `回复` 按钮。
- 点击后在评论输入框里显示“回复 @用户名”，提交时携带 `parent_id`。
- 回复以缩进样式显示在父评论下。

## 后端实现切片

1. 模型：给 `Comment` 增加 `parent_id`，新增 `Notification` 模型。
2. Schema：`CommentCreate.parent_id`，`CommentOut.parent_id/replies/reply_count`，新增 `NotificationOut`。
3. Service：新增 `create_notification()`，统一避免给自己发通知。
4. 评论接口：创建评论后给图纸作者/被回复者发通知。
5. 点赞/收藏接口：成功新增点赞/收藏时给作者发通知。
6. 通知接口：列表、未读数、单条已读、全部已读。
7. 测试：覆盖评论通知、回复通知、点赞/收藏通知、权限隔离、已读状态。

## 前端实现切片

1. `api.js` 增加通知 API。
2. `state` 增加通知未读数和当前回复目标。
3. `renderNavbar()` 增加通知入口和红点。
4. 增加 `renderNotifications()` 页面。
5. `renderDetail()` 评论区增加回复按钮和分组渲染。
6. 登录后刷新未读数，登出清空通知状态。

## 风险与取舍

- **迁移风险**：当前 SQLite 使用 `create_all`，新增字段不会自动 alter 既有表；需要补轻量迁移脚本或引入 Alembic。建议本迭代顺手落 Alembic 基础迁移。
- **通知重复**：点赞/收藏已有唯一约束，只有“新建成功”才发通知；重复操作不发。
- **评论层级复杂度**：MVP 限制一级回复，前端渲染和 API 都简单。
- **实时性**：先 60 秒轮询即可，社区早期够用；用户量上来再考虑 WebSocket/SSE。

## 验收清单

- [ ] 后端测试全部通过。
- [ ] 前端 build 通过。
- [ ] 作者能收到新评论通知。
- [ ] 评论者能收到回复通知。
- [ ] 点赞/收藏只在首次操作时产生通知。
- [ ] 导航栏未读数正确展示。
- [ ] 通知页能标记单条/全部已读。
- [ ] 旧评论列表兼容，无 `parent_id` 的评论正常显示。
