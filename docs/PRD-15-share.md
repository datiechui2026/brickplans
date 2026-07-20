# PRD-15: 分享功能

| 字段 | 内容 |
|------|------|
| 功能 | 一键分享到社交平台 + 复制链接 |
| 优先级 | P1（社交裂变入口） |
| 预估 | 0.25天 |

---

## 1. 产品目标

让用户方便地分享图纸到微信/QQ/微博等平台，利用社交网络获客。

## 2. 交互设计

详情页加入分享按钮：

```
┌──────────────────────┐
│  ❤️ 收藏   🔗 分享   │
└──────────────────────┘
      ↓ 点击
┌────────────────────────────┐
│  分享到：                  │
│  [复制链接] 📋             │
│  [微信] 💬    [QQ] 🐧      │
│  [微博] 📢    [Twitter] 🐦 │
│                           │
│  https://brickplan.cn/   │
│  blueprint/midieval-castle │
└────────────────────────────┘
```

## 3. 实现要点

### 复制链接

```js
navigator.clipboard.writeText(url).then(() => toast('链接已复制'))
```

### 平台分享

全部用 URL scheme 打开，纯前端零依赖：

```js
// 微信（引导用户复制后粘贴到微信）
// QQ
`https://connect.qq.com/widget/shareqq/index.html?url=${url}&title=${title}`
// 微博
`https://service.weibo.com/share/share.php?url=${url}&title=${title}&pic=${image}`
// Twitter
`https://twitter.com/intent/tweet?url=${url}&text=${title}`
```

### 分享参数

当前 SPA 用 hash 路由（`#/detail?id=xxx`），对分享不友好。

**方案**：在 nginx 层做 URL 重写，支持 `/blueprint/{slug}` 格式：
- `/blueprint/midieval-castle` → 内部重写为 `#/detail?id=xxx`
- 或用 JS 在加载时解析 pathname

**MVP 简化**：直接用 hash URL 分享（现代浏览器支持），后续优化。

### 分享卡片数据

后端已有蓝图数据，分享时直接从 `bp.title`、`bp.description`、`bp.images[0].url` 取。

## 4. 验收标准

- [ ] 复制链接按钮可用，复制后有 toast 提示
- [ ] 微博/QQ/Twitter 分享链接生成正确
- [ ] 微信分享引导复制（或显示二维码，可选）
- [ ] 分享链接可正常打开图纸详情

## 5. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `frontend/src/main.js`（分享按钮+弹窗） |
| 修改 | `frontend/src/main.css`（分享弹窗样式） |
