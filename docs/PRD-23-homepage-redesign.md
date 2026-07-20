# PRD-23: 首页重构 + 数据指标完善

## 需求来源
1. 首页热门作品需显示封面图、点赞数、下载量、收藏数 — 目前这些指标不全或缺失
2. 首页布局需重设计：热门推荐更大篇幅、分类按钮缩小、统计数据真实化
3. 首页上传按钮：未登录跳登录/注册页，已登录跳上传页

## 现况问题

### 数据指标缺失
| 指标 | 后端模型 | 列表API | 详情API | 问题 |
|------|:---:|:---:|:---:|------|
| view_count | ✅ | ✅ | ✅ | OK，作为热度指标 |
| favorite_count | ❌(计算) | ❌ | ✅ | 列表需加 |
| like_count | ❌ | ❌ | ❌ | 全新 |
| 平台统计 | ❌ | ❌ | ❌ | 硬编码假数据 |

### 首页布局问题
- 统计数据硬编码 12,847 / 5,230 — 一眼假
- 热门区仅 4 个卡片，无封面逻辑
- 分类按钮用 grid-3 占很大面积
- 卡片显示 `❤️ {view_count}` — 用❤️显示浏览数（bug）

---

## 方案

### 一、后端：新增数据指标

#### 1.1 新增 Like 模型
```python
class Like(Base):
    __tablename__ = "likes"
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uuid4)
    user_id: Mapped[str] = mapped_column(ForeignKey("users.id"), nullable=False)
    blueprint_id: Mapped[str] = mapped_column(ForeignKey("blueprints.id"), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)
    # unique constraint on (user_id, blueprint_id)
```

#### 1.2 Blueprint 模型加字段
```python
# 新增到 Blueprint 模型
like_count: Mapped[int] = mapped_column(Integer, default=0)      # 冗余计数，点赞数
```

#### 1.3 新增 API 端点

**点赞（需登录）**：
```
POST   /api/blueprints/{id}/like     → 增加 like_count，创建 Like 记录
DELETE /api/blueprints/{id}/like     → 减少 like_count，删除 Like 记录
```

**平台统计（公开）**：
```
GET /api/stats → {
    total_blueprints: int,
    total_users: int,
    total_favorites: int,
    total_pieces: int,       # SUM(piece_count)
    total_views: int         # SUM(view_count)
}
```

#### 1.4 列表 API 返回计数

修改 `BlueprintOut` schema 和 `listBlueprints`：
```python
class BlueprintOut:
    ...
    favorite_count: int = 0
    like_count: int = 0
    is_liked: bool = False       # 当前用户是否点赞
    cover_url: str | None = None # 封面图URL
```

列表查询时 JOIN 计算 favorite_count、like_count，获取 is_cover 的图片作为 cover_url。

### 二、前端：首页重设计

#### 2.1 新布局结构（从上到下）

```
┌─────────────────────────────────────────┐
│  🧱 BrickPlan                          │
│  分享你的积木创意 · 搭建无限可能           │
│  [🔍 探索图纸]  [📤 上传作品]             │  ← 上传按钮逻辑见2.2
├─────────────────────────────────────────┤
│  📐 1,247 图纸  👥 89 创作者             │  ← 真实动态数据
│  ❤️ 2.3k 总收藏  🧩 45k 零件             │
├─────────────────────────────────────────┤
│  🔥 热门推荐              [查看全部 →]    │
│  ┌─────────┬─────────┬─────────┐        │
│  │ 封面图   │ 封面图   │ 封面图   │        │  ← 更大卡片，带封面
│  │ 标题     │ 标题     │ 标题     │        │
│  👁123 ❤️45 👍23                         │  ← 显示三维指标
│  └─────────┴─────────┴─────────┘        │
│  ┌─────────┬─────────┬─────────┐        │
│  │ 第二行   │          │          │        │  ← 6个卡片，2行×3列
│  └─────────┴─────────┴─────────┘        │
├─────────────────────────────────────────┤
│  📂 按分类浏览                           │
│  [🏰建筑] [🚗车辆] [🤖机甲] ...          │  ← 缩小为一行按钮组
├─────────────────────────────────────────┤
│  © 2026 BrickPlan                      │
└─────────────────────────────────────────┘
```

#### 2.2 上传按钮逻辑
- **未登录**：跳转 `showModal('login')`（当前是 register，改为 login 更合理）
- **已登录**：`navigate('upload')`
- 注意：此按钮只在首页 hero 区域，导航栏上传按钮已在 PRD-21 移除

#### 2.3 封面图逻辑
卡片取封面图：`bp.cover_url`（API 返回的第一个 is_cover 图，或无封面则取第一张）

#### 2.4 修复卡片 bug
当前：`❤️ ${bp.view_count}` → 改为 `👁 ${bp.view_count} · ❤️ ${bp.favorite_count}`

### 三、首页热门推荐卡片组件

每个卡片展示：
- **封面图**（16:9 比例，`object-fit: cover`）
- **标题**（加粗，最多 20 字截断）
- **作者**（小字，点击进个人主页）
- **指标栏**：`👁 123  ❤️ 45  👍 23`

### 四、涉及文件

| 层级 | 文件 | 改动 |
|------|------|------|
| 模型 | `backend/app/models/__init__.py` | 新增 Like 模型，Blueprint 加字段 |
| Schema | `backend/app/schemas/__init__.py` | BlueprintOut 加 count 字段 |
| API | `backend/app/api/blueprints.py` | 新增 like/download 端点，列表加计数 |
| API | `backend/app/api/stats.py` | 新建平台统计端点 |
| 主程 | `backend/app/main.py` | 注册 stats/like 路由 |
| 前端 | `frontend/src/main.js` | 重写 renderHome，修复卡片 |
| 前端 | `frontend/src/api.js` | 新增 likeBlueprint/getStats 等函数 |

## 验收标准
1. 首页统计数字来自数据库真实数据（非硬编码）
2. 热门推荐展示 6 个作品（2行×3列），每张卡片显示封面图
3. 卡片显示 `👁 浏览  ❤️ 收藏  👍 点赞` 三项真实指标
4. 分类按钮缩小为一行小按钮组
5. 点击 ❤️ 可收藏/取消收藏，点击 👍 可点赞/取消点赞（需登录）
6. 首页上传按钮：未登录→登录弹窗，已登录→上传页
7. 平台统计 API 返回正确数据

## 任务量估算
2 天（后端 1 天 + 前端 1 天）
