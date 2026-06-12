# PRD-18A: 头像体系增强（默认头像 + 预设头像库）

| 字段 | 内容 |
|------|------|
| 功能 | 注册默认头像 + 预设乐高小人头像选择 |
| 优先级 | **P0（产品闭环）** |
| 预估 | 0.5天 |
| 父 PRD | PRD-18 用户设置 & 作品管理 |

---

## 1. 产品目标

当前头像体系有缺口：新用户注册后无头像（只显示首字母圆圈），点击更换只能本地上传。补全：

- **注册时自动分配默认头像**：从预设头像库随机选一个，新用户不再"光头"
- **预设乐高小人头像库**：提供 20 个乐高风格头像供选择
- **头像选择入口升级**：点击更换头像 → 弹出选择器（本地上传 / 预设头像 两个 Tab）

## 2. 用户故事

- 作为新用户，注册完成就有一个酷酷的乐高小人头像，不需要自己上传
- 作为用户，我可以从 20 个乐高风格头像里选一个喜欢的
- 作为用户，我也能上传自己的照片做头像

## 3. 交互流程

### 3.1 头像更换入口

```
点击头像区域（原"点击更换头像"）
        ↓
┌──────────────────────────────┐
│  🎨 更换头像                  │
│                              │
│  ┌─ Tabs ──────────────────┐ │
│  │ [预设头像] [本地上传]     │ │
│  └─────────────────────────┘ │
│                              │
│  【预设头像 Tab】             │
│  ┌──┐ ┌──┐ ┌──┐ ┌──┐ ┌──┐  │
│  │😊│ │😎│ │🤓│ │😇│ │🤩│  │
│  └──┘ └──┘ └──┘ └──┘ └──┘  │
│  ┌──┐ ┌──┐ ┌──┐ ┌──┐ ┌──┐  │
│  │😜│ │😏│ │🥳│ │😤│ │🤔│  │
│  └──┘ └──┘ └──┘ └──┘ └──┘  │
│  ...（共20个乐高小人头像）    │
│    当前选中：黄色边框高亮     │
│                              │
│  【本地上传 Tab】             │
│  📁 点击选择图片              │
│  支持 JPG/PNG/WebP，≤2MB    │
│                              │
│  [取消]           [确认选择]  │
└──────────────────────────────┘
```

### 3.2 注册默认头像

注册时，后端从预设头像中随机选一个赋给 `User.avatar_url`。

## 4. 后端API

### 4.1 预设头像列表

**`GET /api/auth/avatars`**（公开，无需登录）

```json
// Response
{
  "avatars": [
    {"id": "preset-01", "url": "/avatars/presets/01.png"},
    {"id": "preset-02", "url": "/avatars/presets/02.png"},
    ...
  ]
}
```

### 4.2 选择预设头像

**`PUT /api/auth/me`** 扩展支持 `avatar_url` 字段：

```json
// Request - 接受 avatar_url（预设路径或上传路径均可）
{
  "avatar_url": "/avatars/presets/05.png"
}

// Response
{
  "id": "uuid",
  "username": "...",
  "avatar_url": "/avatars/presets/05.png",
  ...
}
```

### 4.3 注册时分配默认头像

`POST /api/auth/register` 成功创建用户后，从预设头像中随机选一个，设置 `avatar_url`。

预设头像路径格式：`/avatars/presets/{01-20}.png`

## 5. 前端实现

### 5.1 settings Modal 改造

当前 `avatarPreview.onclick` 直接触发 file input。改为打开头像选择 Modal。

### 5.2 头像选择 Modal

新函数 `openAvatarPicker()`：
- 两个 Tab：预设头像 / 本地上传
- 预设头像 Tab：调用 `GET /api/auth/avatars` 获取列表，网格展示
- 点击头像高亮选中
- 确认按钮调用 `PUT /api/auth/me` 传 `avatar_url`
- 本地上传 Tab：复用现有 file input + upload 逻辑

### 5.3 个人主页 & 评论 & 导航栏

所有显示头像的地方（已有 `avatar_url` 逻辑），无需改动。确保预设头像 URL 能正常显示即可。

## 6. 静态文件

预设头像存放在 `frontend/public/avatars/presets/`，构建时打包到 dist：
- `frontend/public/avatars/presets/01.png` ~ `20.png`
- Nginx 或 Vite dev server 直接 serve

## 7. 验收标准

- [ ] 新用户注册后自动获得随机乐高小人头像
- [ ] 设置页面点击头像 → 弹出选择器（预设头像 + 本地上传两个Tab）
- [ ] 预设头像网格显示 20 个头像，点击可选中（黄色边框高亮）
- [ ] 选择预设头像后保存成功，全站头像即时更新
- [ ] 本地上传仍然可用
- [ ] 所有显示头像的地方（导航栏/个人主页/评论）正常显示

## 8. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `backend/app/api/auth.py`（register 加默认头像，PUT /me 支持 avatar_url） |
| 修改 | `frontend/src/main.js`（头像选择 Modal + 注册后默认头像显示） |
| 修改 | `frontend/src/main.css`（头像选择器样式） |
| 修改 | `frontend/src/api.js`（加 getPresetAvatars / selectAvatar） |
| 新增 | `frontend/public/avatars/presets/`（20 个乐高小人头像 PNG） |
