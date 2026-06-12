# PRD-01: 图片上传功能

| 字段 | 内容 |
|------|------|
| 功能 | 图纸图片上传到 MinIO |
| 优先级 | P0 |
| 预估 | 0.5天 |
| 依赖 | MinIO 已配置（docker-compose） |

---

## 1. 产品目标

用户创建图纸时能上传图片，上传后在详情页展示缩略图，在卡片中显示封面图。

## 2. 用户故事

- 用户在「上传图纸」页面，填写完标题后，能点击上传区域选择图片
- 支持多张图片上传（最多10张），第一张自动设为封面
- 上传后立即显示缩略图预览
- 图纸卡片不再用 emoji 占位，改用真实封面图
- 图纸详情页展示图片轮播

## 3. 技术方案

### 3.1 后端：图片上传 API

**新建文件**: `backend/app/api/images.py`

```
POST /api/blueprints/{blueprint_id}/images
  接收 multipart/form-data，字段 file（图片）
  上传到 MinIO，写入 blueprint_images 表
  返回 BlueprintImageOut

DELETE /api/blueprints/{blueprint_id}/images/{image_id}
  删除 MinIO 文件 + 数据库记录
```

**新建文件**: `backend/app/services/storage.py`

```
MinIO 客户端封装：
- upload_file(file) → url
- delete_file(object_key)
- 自动创建 bucket（如不存在）
- 文件名：{blueprint_id}/{uuid}.{ext}
```

**修改文件**: `backend/app/main.py`
- `include_router(images.router)`

### 3.2 后端：修复 images 字段返回

**修改文件**: `backend/app/api/blueprints.py`
- `_to_blueprint_out()` 中 `"images": []` 改为读取 `bp.images`
- `get_blueprint()` 已 eager load images，需确保列表页也加载

### 3.3 前端：上传组件

**修改文件**: `frontend/src/main.js` — `renderUpload()` 区域
- 标题上方新增图片上传区：
  - 点击选择文件（`<input type="file" accept="image/*" multiple>`）
  - 拖拽上传
  - 上传中显示进度
  - 上传完成显示缩略图网格
- 先创建图纸（获取 blueprint_id），再逐张调用上传 API

**修改文件**: `frontend/src/api.js`
- 新增 `uploadBlueprintImage(blueprintId, file)` 方法

### 3.4 前端：卡片真实图片

**修改文件**: `frontend/src/main.js` — `renderBlueprintCard()`
- 如果 `bp.images?.length > 0`，用第一张图的 URL 替换 emoji+渐变色
- 保留渐变背景作为 fallback

---

## 4. 验收标准

- [ ] POST `/api/blueprints/{id}/images` 能接收图片并存入 MinIO
- [ ] 上传后 `GET /api/blueprints/{id}` 返回的 images 不为空
- [ ] 上传页面能选择图片、预览缩略图
- [ ] 图片上传到 MinIO 后 URL 可访问
- [ ] 图纸卡片显示真实封面图（有图片时）
- [ ] 不支持的文件类型返回 400

## 5. 涉及文件

| 操作 | 文件 |
|------|------|
| 新建 | `backend/app/api/images.py` |
| 新建 | `backend/app/services/storage.py` |
| 修改 | `backend/app/main.py` |
| 修改 | `backend/app/api/blueprints.py` |
| 修改 | `frontend/src/main.js` |
| 修改 | `frontend/src/api.js` |
