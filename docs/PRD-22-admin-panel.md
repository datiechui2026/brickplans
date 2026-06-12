# PRD-22: 管理员后台

## 现状
目前没有管理员系统。所有蓝图发布后直接上线，无审核流程。没有管理员角色区分。

## 方案设计

### 一、管理员角色

**数据库中新增字段**：`User.is_admin: bool = False`

**设置管理员的方式**（开发阶段）：
- 直接改数据库：`UPDATE user SET is_admin = 1 WHERE email = 'admin@brickplans.com';`
- 后续可做命令行脚本设管理员

### 二、管理员入口

**前端**：管理员登录后，导航栏显示 🔧 管理 入口（仅 admin 可见）
- 点击进入 `/admin` 页面

### 三、管理后台页面

**布局**：左侧导航 + 右侧内容（简洁单页即可）

**功能模块**：

1. **作品审核队列**（顶部 Tab）
   - 显示 `is_published = false` 的蓝图（用户提交但未发布）
   - 卡片列表：缩略图、标题、作者、时间、✅通过 / ❌拒绝 / 🗑删除
   - 通过 → 设 `is_published = true`
   - 拒绝 → 可填写拒绝理由，通知作者（暂不做通知系统的话先跳过）
   - 支持批量操作

2. **全部作品管理**
   - 搜索（标题/作者）
   - 列表：缩略图、标题、作者、发布时间、状态、操作
   - 操作：👁查看 / ✏编辑 / 🚫下架 / 🗑删除
   - 下架 → 设 `is_published = false`
   - 支持分页

3. **用户管理**（可选，P2）
   - 搜索用户、查看用户信息、禁用用户

### 四、后端 API

需要新增的端点（加权限校验 `get_current_admin`）：

```
# 管理API（需要 admin 权限）
GET    /api/admin/blueprints          # 全部作品列表（含未发布）
GET    /api/admin/blueprints/pending  # 待审核列表
PUT    /api/admin/blueprints/{id}/publish    # 通过审核
PUT    /api/admin/blueprints/{id}/unpublish  # 下架
DELETE /api/admin/blueprints/{id}     # 删除（已有，加admin权限）
```

**权限中间件** `get_current_admin`：
- 先调 `get_current_user` 获取用户
- 检查 `user.is_admin`，否则 403

### 五、前端页面

新建管理页面路由 `admin`：
- 仅 `state.user.is_admin` 时可见和可访问
- 审核队列 Tab + 全部作品 Tab
- 搜索框（模糊搜索标题/作者）
- 作品列表卡片 + 操作按钮
- 确认对话框（删除/下架二次确认）

## 验收标准
1. 管理员账号登录后导航栏出现 🔧 管理 入口
2. 管理后台显示全部作品列表，可搜索
3. 待审核 Tab 显示未发布作品，可一键通过/拒绝
4. 已发布作品可下架（设 is_published=false）
5. 可删除任意作品（二次确认）
6. 普通用户看不到管理入口，直接访问 /admin 返回 403 或跳转

## 涉及文件
- `backend/app/models/__init__.py` — User 加 is_admin 字段
- `backend/app/api/admin.py` — 新建管理 API 路由
- `backend/app/api/deps.py` — 新增 get_current_admin 依赖
- `backend/app/main.py` — 注册 admin 路由
- `frontend/src/main.js` — nav 加管理入口、renderAdminPage()、相关交互
- `frontend/src/api.js` — 新增管理 API 函数

## 任务量估算
1.5 天
