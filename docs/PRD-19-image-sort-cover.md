# PRD-19: 图片排序 & 封面图设置

| 字段 | 内容 |
|------|------|
| 功能 | 上传图片拖拽排序 + 设置封面图 + 默认封面逻辑 |
| 优先级 | **P0（产品闭环）** |
| 预估 | 0.5~1天 |
| 背景 | 积木图纸有拼装顺序，图片顺序就是拼装说明书 |

---

## 1. 产品目标

积木图纸的图片不是随机排列的——它们是拼装步骤说明书。必须支持：
1. **拖拽排序**：用户可以调整图片顺序以反映拼装流程
2. **封面设置**：用户可以指定一张图片作为封面
3. **默认封面**：未设置封面时，取最后一张（成品展示图）作为封面

## 2. 用户故事

- 作为创作者，我上传完图片后可以拖拽调整顺序，确保拼装步骤正确
- 作为创作者，我可以指定成品图作为封面，让浏览者一眼看到最终效果
- 作为浏览者，我在首页卡片看到的是成品图，进入详情可以从第一步开始看

## 3. 交互设计

### 3.1 上传页——图片管理

```
┌────────────────────────────────────────────┐
│  📸 图片管理                                │
│                                            │
│  点击选择图片 或 拖拽图片到此处             │
│  支持 JPG / PNG / WebP，每张 ≤10MB         │
│                                            │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐     │
│  │ ⭐封面│ │ 步骤1│ │ 步骤2│ │ 步骤3│     │
│  │ [图] │ │ [图] │ │ [图] │ │ [图] │     │
│  │  ⭐  │ │  ×   │ │  ×   │ │  ×   │     │
│  └──────┘ └──────┘ └──────┘ └──────┘     │
│                                            │
│  💡 拖拽图片可调整顺序                      │
│  💡 点击 ⭐ 可设置封面                      │
│  💡 未设封面时默认最后一张为封面             │
└────────────────────────────────────────────┘
```

- 每张图片右下角有 ⭐ 按钮 → 点击设为封面（选中后高亮变金色）
- 每张图片右上角有 × 删除按钮
- 图片可拖拽排序（HTML5 Drag & Drop API 或使用简单的上下移动按钮）
- 第一张图片默认显示 ⭐ 标记

### 3.2 详情页——轮播适配

轮播起点不再是 `images[0]`，而是封面图（`is_cover=True` 或最后一张）。

### 3.3 编辑作品 Modal

编辑现有作品时，图片管理区域也支持排序+封面设置。

## 4. 后端 API

### 4.1 图片重排序

**`PUT /api/blueprints/{blueprint_id}/images/reorder`**

```json
// Request
{
  "images": [
    {"id": "uuid-1", "sort_order": 0},
    {"id": "uuid-2", "sort_order": 1},
    {"id": "uuid-3", "sort_order": 2}
  ]
}

// Response: 200
{"message": "ok"}
```

### 4.2 设置封面图

**`PUT /api/blueprints/{blueprint_id}/images/{image_id}/cover`**

```json
// Request: 空 body
// 逻辑：先将该 blueprint 下所有图片的 is_cover 设为 false
//       再将指定图片的 is_cover 设为 true

// Response: 200
{"message": "ok"}
```

### 4.3 封面图解析逻辑

后端返回蓝图列表和详情时，图片按 `sort_order` 升序排列。前端取封面逻辑：

```js
// 前端卡片取封面
function getCoverImage(images) {
  if (!images || images.length === 0) return null;
  const cover = images.find(img => img.is_cover);
  if (cover) return cover.url;
  // 默认取最后一张
  return images[images.length - 1].url;
}
```

### 4.4 上传图片时指定 sort_order

**`POST /api/blueprints/{blueprint_id}/images`** 扩展参数，接受可选 `sort_order`。

## 5. 前端实现

### 5.1 上传页改造 (`main.js`)

- `handleImageSelect()` → 预览区改为可拖拽网格
- 每个预览卡片上加 ⭐ 按钮（设为封面）
- 拖拽排序后更新 `_selectedFiles` 数组顺序
- 上传时传入 `sort_order`（按数组索引）

### 5.2 拖拽排序实现

使用 HTML5 Drag & Drop API：
- 每张预览图设置 `draggable="true"`
- `dragstart` / `dragover` / `drop` 事件处理
- 拖拽结束后重新渲染预览数组

或者用更简单的方案：每张图加 ⬆️ ⬇️ 按钮来移动位置。

### 5.3 首页卡片

`renderBlueprintCard()` 改用 `getCoverImage(bp.images)` 而不是 `bp.images[0]`。

### 5.4 详情页轮播

`loadDetail()` 中的轮播起始索引改为封面图位置。

### 5.5 CSS

- 封面图标记样式 `.cover-badge`
- 拖拽排序时的视觉反馈

## 6. 验收标准

- [ ] 上传页图片预览区支持拖拽排序
- [ ] 图片上有点击 ⭐ 设为封面的按钮（选中状态金色高亮）
- [ ] 未设封面时，首页卡片使用最后一张图片作为封面
- [ ] 设了封面后，首页卡片使用封面图
- [ ] 详情页轮播从封面图开始展示
- [ ] 编辑作品 Modal 也支持图片排序+封面设置
- [ ] 后端 reorder 和 cover API 正常工作

## 7. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `backend/app/api/blueprints.py`（新增 reorder + cover API） |
| 修改 | `frontend/src/main.js`（拖拽排序 + 封面标记 + 封面取图逻辑） |
| 修改 | `frontend/src/main.css`（封面标记 + 排序样式） |
| 修改 | `frontend/src/api.js`（新增 reorderImages / setCover） |
