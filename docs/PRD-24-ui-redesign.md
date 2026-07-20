# PRD-24：全站 UI 重构 — 统一设计语言

> **产品目标**：将 BrickPlan 全线 6 个页面统一为现代化、一致的设计语言，告别零散的旧样式。
> **关联 PRD**：PRD-23（首页重构 + 点赞系统）将被本 PRD 覆盖替代，本 PRD 一次性完成全部 UI 改造。

---

## 一、设计系统

### 1.1 全局 CSS 变量

```css
:root {
  --bg:        #f5f5f7;   /* 页面背景 */
  --card-bg:   #ffffff;   /* 卡片/面板背景 */
  --text:      #1d1d1f;   /* 主文字 */
  --text-sec:  #86868b;   /* 次要文字 */
  --accent:    #ff6b35;   /* 主色调（橙色） */
  --accent-hv: #e55a2b;   /* hover 加深 */
  --radius:    16px;      /* 圆角 */
  --shadow:    0 1px 4px rgba(0,0,0,.04);      /* 默认阴影 */
  --shadow-hv: 0 8px 32px rgba(0,0,0,.08);     /* hover 阴影 */
  --input-bg:  #f5f5f7;   /* 输入框背景 */
}
```

### 1.2 字体

```css
font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
```

自然，使用系统字体栈，无需引入外部字体文件。

### 1.3 难度色标

| 难度值 | 显示文本 | 颜色 | 背景色（毛玻璃） |
|--------|---------|------|-----------------|
| 1 | ⭐ 简单 | #22c55e | rgba(34,197,94,.75) |
| 2 | ⭐⭐ 初级 | #3b82f6 | rgba(59,130,246,.75) |
| 3 | ⭐⭐⭐ 中等 | #f59e0b | rgba(245,158,11,.75) |
| 4 | ⭐⭐⭐⭐ 困难 | #ef4444 | rgba(239,68,68,.75) |
| 5 | ⭐⭐⭐⭐⭐ 专家 | #a855f7 | rgba(168,85,247,.75) |

**前端实现**：创建一个 `formatDifficulty(n)` 工具函数，返回 `{stars, label, color, bg}`。

---

## 二、导航栏改造

### 现状
- 旧样式导航栏，使用 `var(--brick-*)` 旧变量
- 导航链接：「🏠 首页」「🔍 发现」+ 管理员可见「🔧 管理」

### 改后

```
┌──────────────────────────────────────────────┐
│ 🧱 BrickPlan              📤 上传图纸  👤   │
└──────────────────────────────────────────────┘
```

**规格**：
- 高度 56px，顶部 sticky，z-index 100
- 毛玻璃效果：`background: rgba(255,255,255,.72); backdrop-filter: blur(20px);`
- 底部 1px 边框 `rgba(0,0,0,.06)`
- 品牌 Logo：左侧 `🧱 BrickPlan`，字号 18px，字重 700，无下划线
- 上传按钮：`btn-ghost` 样式，未登录点它 → 跳登录/注册页弹窗
- 用户头像：32px 圆形，点击 → 个人主页
- **去掉旧的导航链接按钮**（首页/发现/管理），全部改为只在 `renderNavbar` 中渲染品牌+上传+头像
- 管理员入口保留但不占用导航空间：管理员登录后，头像下拉或个人主页放「管理后台」入口

> ⚠️ 注意：当前路由依赖导航栏的 `navigate()` 跳转。保留 `navigate` 逻辑但不再显式展示首页/发现链接——用户通过浏览器返回、Logo点击、或卡片点击来导航。Logo 点击 → `#/home`。

---

## 三、卡片组件重写

### 现状

```js
function renderBlueprintCard(bp) {
  // 旧逻辑：backgroundImage CSS + emoji兜底
  // 无难度/零件数
  // 旧 class 名（card clickable）
  // 显示 ❤️ 当浏览数（bug）
}
```

### 改后

卡片在所有页面（首页、发现页、个人主页）复用同一个组件。

**HTML 结构**：

```html
<div class="card" onclick="navigate('detail', {id: bp.id})">
  <!-- 封面图区域 -->
  <div class="card-img-wrap">
    <img class="card-img" src="{cover_url}" 
         onerror="this.style.display='none';this.parentElement.style.background='...渐变...'">
    <!-- 难度标签（左上角毛玻璃） -->
    <span class="card-diff">{formatDifficultyLabel}</span>
    <!-- 零件数（右上角白色半透明） -->
    <span class="card-parts">🧩 {piece_count}片</span>
  </div>
  <!-- 信息区 -->
  <div class="card-body">
    <div class="card-title">{title}</div>
    <div class="card-author">
      <img src="{avatar}"> {username}
    </div>
    <div class="card-stats">
      <span>👁 {view_count}</span>
      <span>❤️ {like_count}</span>
      <span>⭐ {favorite_count}</span>
    </div>
  </div>
</div>
```

**CSS 关键规则**：
- `.card` — 白色背景，圆角 16px，overflow:hidden，hover 上浮 6px + 加深阴影
- `.card-img-wrap` — aspect-ratio: 4/3，overflow:hidden
- `.card-img` — object-fit: cover，hover 放大 1.06x（transition 0.4s）
- `.card-diff` — position:absolute top:10px left:10px，圆角 20px，毛玻璃背景
- `.card-parts` — position:absolute top:10px right:10px，白底半透明
- `.card-title` — 14px 700，最多2行截断
- `.card-stats` — 12px #999，flex gap

**无图降级**：当 `cover_url` 为 null 时，`card-img-wrap` 显示 emoji + 渐变色背景（保留旧逻辑但改为新样式）。

**个人主页特化**：自己的作品卡片在 `.card-body` 右下角追加编辑/删除按钮。

---

## 四、各页面改造

### 4.1 首页（`renderHome`）

**去掉**：
- 旧的 hero section 全部内容（大标题、副标题、两个按钮）
- 旧的 stat-card 样式

**保留**：
- `api.getStats()` 获取统计数据的逻辑（复用）
- 分类筛选功能（改造为 pills）
- 加载更多 / 分页

**新增**：

1. **统计条**：白底卡片内 4 格横排
   ```
   ┌──────────┬──────────┬──────────┬──────────┐
   │    27    │    12    │    62    │    2     │
   │  📐 图纸 │  👥 创作者│  👁 浏览 │  ❤️ 收藏 │
   └──────────┴──────────┴──────────┴──────────┘
   ```
   - 数字：28px 800 橙色
   - 标签：13px 灰色
   - 网格间用 1px 竖线分隔

2. **分类 Pills**：水平排列，圆角 20px
   ```
   [全部] [🏗️建筑] [🤖机甲] [🚗车辆] [🛸科幻] [🎭场景] [🐉奇幻]
   ```
   - 默认态：白底灰边框灰字
   - 选中态：黑底白字（`var(--text)`）
   - 点击切换分类，重新请求列表

3. **四个统计指标映射**（`getStats()` 返回）：
   - `total_blueprints` → 📐 图纸作品
   - `total_users` → 👥 创作者
   - `total_views`（sum of view_count）→ 👁 总浏览量
   - `total_favorites` → ❤️ 总收藏

4. **卡片网格**：4 列 `grid-template-columns: repeat(4, 1fr)`，gap 16px

5. **底部加载更多**按钮

---

### 4.2 发现页（`renderExplore`）

**改造**：
- 搜索栏 + 分类/排序下拉 + 筛选按钮 放在一个白色卡片内（`filter-bar`）
- 热门标签 pills 保留但更新样式
- 卡片网格复用新卡片组件
- 底部分页器：圆角 10px 方块按钮，选中态黑底白字

**功能不变**：搜索、分类、排序、标签筛选逻辑全部保留。

---

### 4.3 详情页（`renderDetail`）

**布局**：左右两栏
- **左栏**（flex: 1）：图片画廊
  - 主图区域（4:3 比例）+ 左右箭头切换
  - 底部圆点指示器
  - 缩略图条（横向滚动）
- **右栏**（380px 固定宽度）：信息区
  - 标题（22px 800）+ 作者行（头像+昵称+发布日期）
  - 4 格元数据：难度 / 零件数 / 分类 / 尺寸
  - 3 个操作按钮（横排等宽）：❤️点赞 / ⭐收藏 / 👁浏览数（只读）
  - ~~下载按钮~~（已移除）

**下方（左栏下方跨全宽）**：
- 作品简介卡片
- 评论区卡片（输入框 + 评论列表）

**响应式**：≤900px 切换为单列布局。

---

### 4.4 上传页（`renderUpload`）

**改造**：
- 表单居中，最大宽度 720px
- 白色卡片包裹所有表单项
- 图片上传区：虚线边框 + 居中提示文字 + 预览缩略图网格
- 分类/难度并排，零件数/尺寸并排（双列 grid）
- 标签：Chips 样式输入
- 发布按钮：橙色大按钮 `btn-primary btn-lg`
- 保存草稿：ghost 按钮

**功能逻辑不变**：图片选择、上传、表单验证、提交逻辑全部保留。

---

### 4.5 个人主页（`renderUserProfile`）

**改造**：
- 头部卡片：头像（88px 圆）+ 昵称 + 加入时间 + 简介 + 操作按钮行
- 编辑资料按钮 → 展开资料编辑面板（头像选择、昵称、简介输入）
- 3 格统计：作品数 / 收藏数 / 获浏览量
- Tabs：📐 我的作品 / ⭐ 我的收藏（橙色下划线选中态）
- 卡片网格复用新卡片组件
- 自己的作品卡片右下角显示 ✏️编辑 / 🗑️删除 小按钮

---

### 4.6 管理后台（`renderAdminPage`）

**改造**：
- 导航栏显示「🧱 BrickPlan **管理**」橙色标签
- 「← 返回前台」链接
- 4 格统计：待审核（橙色数字，warn态）/ 总作品 / 用户数 / 总浏览
- Tabs：⏳待审核 / 📐全部作品 / 👥用户管理
- 搜索框 + 表格（封面缩略图 / 标题 / 作者 / 分类 / 难度 / 时间 / 操作按钮）
- 分页器

**功能逻辑**：审核通过/拒绝、下架、删除等功能全部保留，只改样式。

---

## 五、后端配套改动

| 改动项 | 说明 |
|--------|------|
| `GET /api/blueprints` 列表接口 | 确保返回 `images` 数组和 `cover_url` 字段（当前返回空数组需修复） |
| `GET /api/blueprints` 列表接口 | 确保返回 `like_count`、`favorite_count` 字段（供卡片 stats 使用） |
| `GET /api/blueprints` 列表接口 | 确保返回 `difficulty`、`piece_count`（已有） |
| `GET /api/stats` | 确保返回 `total_blueprints`、`total_users`、`total_views`、`total_favorites`（已有） |

> 后端主要修复：**列表查询需 eager load 图片关联**，使列表中的每个 blueprint 都带上 images 数据和 correct cover_url。

---

## 六、技术实现要点

1. **一个卡片组件**：`renderBlueprintCard(bp)` 在所有列表页面复用
2. **一个难度工具函数**：`formatDifficulty(n)` 返回 `{stars, label, color, bg}`
3. **CSS 类名统一**：所有旧变量 `var(--brick-*)` 替换为新变量
4. **不引入外部依赖**：纯 CSS + 原生 JS，无框架
5. **响应式**：
   - ≥1024px：4 列
   - 768-1023px：3 列
   - 480-767px：2 列
6. **旧样式清理**：移除不再使用的 CSS 规则（hero、stat-card 旧样式等）

---

## 七、验收标准

- [ ] 全局 CSS 变量生效，所有页面背景为 `#f5f5f7`
- [ ] 导航栏毛玻璃效果，Logo 点击回首页
- [ ] 上传按钮未登录跳登录，已登录跳上传页
- [ ] 卡片在所有页面展示统一：封面图 + 难度色标 + 零件数 + 标题 + 作者 + 三指标
- [ ] 首页统计条显示 4 个真实数据（非假数据）
- [ ] 首页分类 pills 点击可切换筛选
- [ ] 详情页图片轮播正常工作（左右箭头 + 缩略图点击）
- [ ] 详情页右栏元数据、操作按钮正确显示
- [ ] 详情页**无下载按钮**
- [ ] 上传页表单布局正常，功能逻辑不变
- [ ] 个人主页编辑资料可展开/收起
- [ ] 管理后台表格样式正常，审核功能不变
- [ ] 响应式在不同宽度下布局正确
- [ ] 无图作品显示 emoji + 渐变色占位
- [ ] `npm run build` 构建成功，dist 正常部署

---

## 八、设计参考

所有页面的 HTML 视觉稿位于：
```
sketches/003-homepage/index.html   — 首页
sketches/004-explore/index.html    — 发现页
sketches/005-detail/index.html     — 详情页
sketches/006-upload/index.html     — 上传页
sketches/007-profile/index.html    — 个人主页
sketches/008-admin/index.html      — 管理后台
```

在线预览：`http://124.221.85.247:8765/sketches/0XX-xxx/index.html`

开发时请参照这些 HTML 文件中的结构和样式（CSS 变量、类名、布局方式），保持像素级一致。
