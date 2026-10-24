# 英文博客内容流水线（EN content pipeline）

英文站（`/en/*`）的博客不是翻译层，而是**独立的英文内容树**：
`backend-go/content/en/blog/<slug>.md`。中文树是 `backend-go/content/blog/`。

## 与 SEO/GEO 的关系

- 两棵树**共用 slug**。`blog.HasLang(slug, "en")` 为真时，`/blog/<slug>` 与
  `/en/blog/<slug>` 才会互相输出 hreflang（`en` / `zh-CN` / `x-default`），
  并各自进入 `sitemap.xml` / `sitemap-en.xml`。slug 不一致 = 永远配不成对。
- `sitemap-en.xml` 与 `sitemap-en-blog-<bucket>.xml` **自动**取自英文树
  （`blog.Store.AllByLang("en")`），新增 `.md` 文件后无需改任何 Go 代码。
- 英文稿缺失时，`/en/blog/<slug>` 会回退渲染中文正文，canonical 指回中文页、
  不输出 hreflang、不进 sitemap-en —— 不产生重复内容，但也不会有英文收录。
- 英文分类必须是 `backend-go/internal/blog/categories.go` 里登记的英文
  category 之一（`Beginner Guide` / `Brand Guide` / `Parts Guide` /
  `Building Techniques` / `Set Review` / `MOC Review` / `MOC Tutorial` /
  `Knowledge` / `Resources`），否则会落进 `other` 桶。

## 流水线

`scripts/en_rewrite.py`：读取中文稿 → 调用 Anthropic 兼容接口重写成英文 →
写 `content/en/blog/<slug>.md`。**逐 slug 幂等**：已存在的英文文件跳过，
中断后重跑即续。

```bash
# 进度
python scripts/en_rewrite.py --stats

# 校验整棵英文树（不调用 API）：CJK 残留、正文长度、图片丢失、分类合法性、frontmatter
python scripts/en_rewrite.py --verify

# 小批量 / 指定文章
python scripts/en_rewrite.py --limit 20 --concurrency 6
python scripts/en_rewrite.py --slug brick-parts-buy-guide --force

# 全量补齐（可后台跑，约 34 篇/分钟 @ concurrency 6）
nohup python scripts/en_rewrite.py --concurrency 6 > /tmp/en_rewrite.log 2>&1 &
```

环境变量：`ANTHROPIC_API_KEY`（必需）、`ANTHROPIC_BASE_URL`、`ANTHROPIC_MODEL`。

## 质量约束（脚本内置校验，失败自动重试一次）

- 输出中**不允许出现任何中文字符**（品牌/型号需要英文写法）。
- 保留源文的事实、数字、零件编号、尺寸、步骤顺序；不允许编造产品/价格。
- 保留源文全部图片 URL（只翻译 alt），内部链接自动改写为 `/en/blog/...`、
  `/en/tags/...`。
- 正文 ≥ 400 词、以 H1 开头；title ≤ 75 字符；description 60–180 字符；
  2–8 个英文 tag。
- `date` 沿用中文稿日期（保持 hreflang 对的发布时间线一致）；
  `author` 统一 `BrickPlan Team`。

## 发布建议

1. 先 `--verify` 全绿，再 `go test ./...`。
2. 内容文件是启动时加载的：部署后需重启 `brickplans-backend`，sitemap 自动更新。
3. 一次性新增上千条 URL 时，观察 GSC「已编入索引」速率；如发现抓取预算被
   挤占，可按主题桶（`sitemap-en-blog-<bucket>.xml`）分批上线。
4. 新英文 URL 在 `scripts/brickplan_indexing_api.py` 的提交队列里天然优先
   （英文 URL 置顶），但 Indexing API 有 200 条/日配额，长尾仍靠 sitemap 抓取。
