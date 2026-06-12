# PRD-21: 编辑页增强 + 导航栏优化

## 需求来源
用户反馈三个问题合并处理：
1. 上传后的编辑页缺少零件数、尺寸、继续上传图片功能
2. 上传按钮和退出按钮应该在个人主页而非导航栏
3. 封面图设置和图片排序需要在上传后的编辑中可用

## 现状分析
- ✅ 编辑弹窗已有：标题、描述、分类、难度、图片排序(▲▼)、封面图(⭐)
- ❌ 缺少：零件数(piece_count)、尺寸(dimensions)、上传更多图片
- ❌ 上传按钮在导航栏（登录后显示）
- ❌ 退出按钮在导航栏右上角

## 方案

### 一、编辑弹窗补充字段

在现有 `openEditBlueprint()` 弹窗的表单中，在难度选择下方添加两个字段：

```
零件数: <input type="number" id="edit-bp-pieces" value="bp.piece_count">
尺寸:   <input type="text" id="edit-bp-dimensions" value="bp.dimensions" placeholder="例如: 30x20x15 cm">
```

`handleSaveEditBlueprint()` 需要把这两个字段加入 `api.updateBlueprint()` 的 payload：
```
piece_count: parseInt(piecesVal) || undefined
dimensions: dimensionsVal || undefined
```

### 二、编辑弹窗添加"上传更多图片"

在图片管理区域底部加一个上传按钮：

```
[📤 添加图片]
```

点击后弹出文件选择器（accept image/*），支持多选。上传流程：
1. 选中文件 → 逐个调 `api.uploadBlueprintImage(id, file)`
2. 每上传成功一张 → append 到 `_editImages` 数组
3. 重新渲染 `renderEditImages()`
4. 显示进度提示"已添加 N 张图片"

注意：部分上传失败时提示"X 张上传成功，Y 张失败"。

### 三、导航栏改造：迁移上传和退出按钮到个人主页

**导航栏变更：**
- 移除 📤 上传按钮（登录状态）
- 移除 "退出" 按钮
- 登录状态保留：搜索框 + 用户名(点击进个人主页)

**个人主页变更（`renderUserProfile`）：**
- 在自己的个人主页顶部，头像旁边添加两个按钮：
  ```
  [📤 上传作品]  [🚪 退出登录]
  ```
- 只有访问自己的主页时才显示（`isOwnProfile === true`）
- 上传按钮：`onclick: () => navigate('upload')`
- 退出按钮：保持原有 `handleLogout` 逻辑

### 四、上传完成后自动跳转

上传蓝图成功后，不再只提示"上传成功"，而是：
1. 跳转到该蓝图的详情页，让用户立即看到效果
2. 或者提示"上传成功！可在个人主页编辑"并留在上传页

**推荐**：跳转到详情页，详情页上方显示一个 3 秒的绿色提示条"🎉 发布成功！可在个人主页管理你的作品"。

## 验收标准
1. 编辑弹窗能修改零件数、尺寸，保存后生效
2. 编辑弹窗能上传新图片，上传后出现在图片列表并可排序/设封面
3. 导航栏不再显示上传按钮和退出按钮（登录状态）
4. 个人主页（自己的）显示 📤上传作品 和 🚪退出登录 按钮
5. 上传作品成功后跳转到详情页（而非留在上传页）
6. 未登录用户点击上传 → 依然弹出登录引导（保持现有一致）

## 涉及文件
- `frontend/src/main.js` — openEditBlueprint()、handleSaveEditBlueprint()、renderNavbar()、renderUserProfile()、renderUpload() 的保存逻辑
- 可能需要 `frontend/src/api.js` 确认 updateBlueprint 接受 piece_count/dimensions（应已支持）

## 任务量估算
1 天
