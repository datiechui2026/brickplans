# PRD-14: SEO 基础

| 字段 | 内容 |
|------|------|
| 功能 | Open Graph + JSON-LD 结构化数据 + sitemap |
| 优先级 | P0（自然流量入口） |
| 预估 | 0.25天 |

---

## 1. 产品目标

搜索引擎能正确抓取和展示网站内容，为自然增长提供基础。

## 2. 实现要点

### 2.1 index.html —— 默认 OG 标签

```html
<meta property="og:title" content="BrickPlan — 积木图纸分享社区" />
<meta property="og:description" content="分享你的乐高MOC创意，探索海量积木图纸" />
<meta property="og:image" content="https://brickplan.cn/og-default.png" />
<meta property="og:url" content="https://brickplan.cn" />
<meta property="og:type" content="website" />
<meta name="description" content="BrickPlan 积木图纸分享社区，发现和分享乐高MOC创意作品" />
```

### 2.2 详情页 —— 动态 OG（服务端渲染）

每个图纸详情页需要独立的 meta 标签。当前是纯前端 SPA，方案：

**方案 A（推荐）**：用前端 `Helmet.js` 类似方式动态改 `<meta>`：
```js
// main.js loadDetail() 中
document.title = `${bp.title} — BrickPlan`;
setMeta('og:title', bp.title);
setMeta('og:description', bp.description);
setMeta('og:image', bp.images?.[0]?.url || '/og-default.png');
```

⚠️ 注意：SPA 改 meta 对搜索引擎抓取有限，但 Google 现在能渲染 JS。

**方案 B**：Nginx 层对爬虫 UA 做 SSR 代理（后续迭代）

### 2.3 sitemap.xml

后端生成静态 sitemap：
```
GET /sitemap.xml → 返回所有公开图纸的 URL 列表
```

### 2.4 robots.txt

```
User-agent: *
Allow: /
Sitemap: https://brickplan.cn/sitemap.xml
```

### 2.5 JSON-LD 结构化数据（详情页）

```html
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "CreativeWork",
  "name": "中世纪城堡",
  "description": "...",
  "image": "...",
  "author": { "@type": "Person", "name": "作者" }
}
</script>
```

## 3. 验收标准

- [ ] index.html 有完整 OG 标签和 meta description
- [ ] 详情页动态修改 title+OG
- [ ] /sitemap.xml 可访问
- [ ] robots.txt 可访问
- [ ] JSON-LD 结构化数据正确

## 4. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `frontend/index.html` |
| 新建 | `frontend/public/robots.txt` |
| 新建 | `backend/app/api/seo.py`（sitemap） |
| 修改 | `frontend/src/main.js`（动态meta） |
| 修改 | `backend/app/main.py`（注册路由） |
