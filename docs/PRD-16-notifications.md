# PRD-16: 通知系统

| 字段 | 内容 |
|------|------|
| 功能 | 站内通知：评论/收藏/关注提醒 |
| 优先级 | P1（用户召回核心） |
| 预估 | 1天 |

---

## 1. 产品目标

用户之间的互动（评论、收藏）需要被感知。通知是用户回头的核心驱动力——"有人评论了你的作品"是最高效的召回钩子。

## 2. 交互设计

### 导航栏——通知铃铛

```
┌────────────────────────────────────┐
│ 🧱 BrickPlan   🔍  [上传] [🔔3]  │  ← 红点显示未读数
└────────────────────────────────────┘
                    ↓ 点击
┌────────────────────────────┐
│ 📬 通知（3条未读）         │
│                           │
│ 🗨 张三 评论了 你的图纸     │
│   「中世纪城堡」            │
│   2分钟前                  │
│ ──────────────────────    │
│ ❤️ 李四 收藏了 你的图纸     │
│   「F1方程式赛车」          │
│   15分钟前                 │
│ ──────────────────────    │
│ 🗨 王五 评论了 你的图纸     │
│   「太空基地」              │
│   1小时前                  │
│                           │
│ [全部已读]                 │
└────────────────────────────┘
```

### 通知类型

| 类型 | 触发事件 | 文案模板 |
|------|---------|---------|
| `comment` | 有人评论了你的图纸 | 「{用户} 评论了你的图纸「{标题}」」 |
| `favorite` | 有人收藏了你的图纸 | 「{用户} 收藏了你的图纸「{标题}」」 |
| `reply` | 有人回复了你的评论 | 「{用户} 回复了你的评论」 |

## 3. 后端实现

### 模型

```python
class Notification(Base):
    id: UUID
    user_id: UUID → 接收通知的用户
    type: str → "comment" | "favorite" | "reply"
    actor_id: UUID → 触发者
    blueprint_id: UUID? → 关联图纸
    comment_id: UUID? → 关联评论
    is_read: bool = False
    created_at: datetime
```

### API

```
GET  /api/notifications         → 我的通知列表（分页）
GET  /api/notifications/unread-count → 未读数
POST /api/notifications/read-all     → 全部已读
POST /api/notifications/{id}/read   → 单条已读
```

### 触发时机

在现有 API 中插入通知创建：
- `POST /api/blueprints/{id}/comments` → 创建通知给图纸作者
- `POST /api/blueprints/{id}/favorite` → 创建通知给图纸作者
- 回复评论 → 创建通知给被回复者

### 不通知自己

自己评论/收藏自己的图纸不创建通知。

## 4. 前端实现

### 导航栏

- 加入铃铛图标 `🔔`
- 红点显示未读数（轮询或 SSE）
- 轮询频率：30秒

### 通知面板

- 下拉面板展示最近通知
- 点击跳转到对应图纸详情
- "全部已读"按钮

### 未读计数

```js
// 每30秒拉一次
setInterval(async () => {
  const { count } = await api.get('/notifications/unread-count');
  updateBell(count);
}, 30000);
```

## 5. 验收标准

- [ ] 收到评论时通知创建正确
- [ ] 收到收藏时通知创建正确
- [ ] 导航栏铃铛显示未读计数
- [ ] 点击铃铛展开通知面板
- [ ] 点击通知跳转到图纸详情
- [ ] "全部已读"正常
- [ ] 自己操作自己不会收到通知

## 6. 涉及文件

| 操作 | 文件 |
|------|------|
| 新建 | `backend/app/models/__init__.py`（Notification 模型） |
| 新建 | `backend/app/api/notifications.py` |
| 修改 | `backend/app/api/blueprints.py`（评论/收藏时发通知） |
| 修改 | `backend/app/main.py`（注册路由） |
| 修改 | `frontend/src/main.js`（通知铃铛+面板） |
| 修改 | `frontend/src/main.css`（通知样式） |
