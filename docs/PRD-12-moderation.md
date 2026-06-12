# PRD-12: 内容审核与举报

| 字段 | 内容 |
|------|------|
| 功能 | 轻量审核流程 + 举报机制 |
| 优先级 | P0（上线前必须有） |
| 预估 | 0.5天 |

---

## 1. 产品目标

内容社区必须有基本的审核兜底。MVP不做全人工审核，用**先发后审+用户举报**模式，上线即可运营。

## 2. 审核流程

```
用户上传 → is_published=False（草稿状态）
         → 前端用户端可见（自己的"我的作品"列表）
         → 首页/发现页不展示
         → 管理后台可审 → is_published=True → 公开可见
```

**轻量方案**：先跳过人工审核步骤，上传即发布。保留 is_published 字段和切换开关。后续有运营后台再加审核。

### 实际 MVP 落地

```
上传 → is_published=auto → 直接展示
     → 用户举报 → 管理员下架（is_published=False）
```

## 3. 举报功能

### 前端——详情页底部

```
🚩 举报此内容
  └── 点击弹出理由选择：
      ○ 内容不当
      ○ 版权问题
      ○ 垃圾广告
      ○ 其他
      📝 （可选）补充说明
      [提交举报]
```

### 后端——举报 API

**`POST /api/reports`**
```json
{
  "blueprint_id": "uuid",
  "reason": "inappropriate",
  "detail": "可选的补充说明"
}
```

**模型**：
```python
class Report(Base):
    id
    reporter_id → 举报人
    blueprint_id → 被举报图纸
    reason → 原因枚举
    detail → 补充说明
    status → "pending" / "resolved" / "dismissed"
    created_at
```

### 举报限制

- 同一用户对同一图纸只能举报一次
- 不暴露举报人身份

## 4. 管理端（最小可行）

在数据库层面支持 `is_published` 切换：
- 当某图纸收到 ≥3 个举报时自动隐藏
- 管理员可通过 SQL 直接操作（后续做后台）

## 5. 验收标准

- [ ] `is_published=False` 的图纸不出现在首页/发现页
- [ ] `is_published=False` 的图纸出现在作者"我的作品"中（标记"审核中"）
- [ ] 上传后默认 `is_published=True`（先发后审模式）
- [ ] 详情页有举报入口
- [ ] 举报 API 可提交且去重
- [ ] 3次举报自动下架

## 6. 涉及文件

| 操作 | 文件 |
|------|------|
| 新建 | `backend/app/models/__init__.py`（加 Report 模型） |
| 新建 | `backend/app/api/reports.py` |
| 修改 | `backend/app/api/blueprints.py`（is_published 过滤） |
| 修改 | `frontend/src/main.js`（举报按钮+弹窗） |
| 修改 | `backend/app/main.py`（注册路由） |
