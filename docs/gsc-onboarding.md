# Google Search Console (GSC) 接入操作单 — brickplan.cn

- 站点：https://brickplan.cn （生产服 8.217.63.46 / nginx 443 → Go 后端 127.0.0.1:8100）
- 编写：开发02AI，2026-10-14
- 状态：**服务器侧已全部就绪并实测通过**；只等站长在 GSC 控制台点几下（约 3 分钟）

---

## 〇、先看结论（站长 3 步）

| 步骤 | 你要做什么 | 预计耗时 |
|---|---|---|
| 1️⃣ 验证资源 | GSC 加资源 `https://brickplan.cn/` → 选「HTML 文件」→ **把 GSC 给出的 `googleXXXXXXXX.html` 文件名发到群里** → 收到「已就绪」后点「验证」 | 2 分钟 |
| 2️⃣ 提交 sitemap | GSC「站点地图」→ 输入 `sitemap.xml` → 提交 | 30 秒 |
| 3️⃣ 看收录报告 | GSC「索引 → 网页」，按 sitemap 维度看 12 个子 sitemap 各自的收录数 | 随时 |

> ⚠️ **为什么第 1 步要发文件名**：GSC 的 HTML 验证文件名是 **Google 为你的账号实时生成** 的（`google` + 16 位随机串），无法预先猜出。我们把服务器侧做成了「文件一放就通」——你发文件名，我们 30 秒内落盘，你直接点验证即可。

---

## 一、服务器侧已完成（本次交付）

### 1. 验证文件存放位置（持久目录，不怕前端重新构建）

| 项目 | 值 |
|---|---|
| 目录（生产） | `/etc/nginx/seo-keys/` |
| 目录（预发布/本机） | `/etc/nginx/seo-keys/`（已同步创建） |
| 已就位占位文件 | `/etc/nginx/seo-keys/googlea83021971b8ab084.html` |
| 文件内容格式 | `google-site-verification: googleXXXXXXXX.html`（一行，无换行） |
| 对外 URL | `https://brickplan.cn/googleXXXXXXXX.html` |
| nginx location | `location ~ ^/google[A-Za-z0-9_-]+\.html$ { root /etc/nginx/seo-keys; default_type text/html; }` |

**实测证据（2026-10-14 09:27）**

```
GET https://brickplan.cn/googlea83021971b8ab084.html
→ HTTP 200
→ 内容: google-site-verification: googlea83021971b8ab084.html

反证 GET https://brickplan.cn/google1791941224zzzz.html（未放置的名字）
→ HTTP 404   ✅ 只有我们放置的 token 才会被放行
```

> 🔒 反证是有意为之：如果任意 `google*.html` 都返回 200，第三方就能在**自己的** GSC 里验证 brickplan.cn，进而看到本站的搜索表现数据。所以这里只服务已放置的文件。

### 2. 一键安装脚本（收到 GSC 文件名后执行）

```bash
# 用法一：完整文件名
/home/ubuntu/.hermes/scripts/gsc_verify_setup.sh google1a2b3c4d5e6f7a8b.html
# 用法二：只给 token
/home/ubuntu/.hermes/scripts/gsc_verify_setup.sh 1a2b3c4d5e6f7a8b
# 体检（不安装，只看现状）
/home/ubuntu/.hermes/scripts/gsc_verify_setup.sh --check
```

脚本做的事（幂等）：校验文件名 → 写入 `/etc/nginx/seo-keys/` → 自动补齐 nginx location（自动备份配置、`nginx -t` 通过后 reload）→ 线上断言 HTTP 200 且内容逐字节匹配 → 反证未放置的文件必须 404。

### 3. sitemap 已按分类拆分（2026-10-14 上线，同时利于 GSC 分主题看收录）

根 `https://brickplan.cn/sitemap.xml` 现在是 **sitemap index**，指向 12 个子 sitemap：

| 子 sitemap | 内容 | URL 数（2026-10-14） |
|---|---|---|
| `sitemap-pages.xml` | 首页/探索/FAQ/关于/隐私 + 6 个分类页 + 标签页 + 博客列表及分页 | 42 |
| `sitemap-blueprints.xml` | 图纸详情页（仅已发布） | 1950 |
| `sitemap-blog-brand.xml` | 品牌推荐 | 144 |
| `sitemap-blog-beginner.xml` | 新手教程 | 518 |
| `sitemap-blog-parts.xml` | 零件知识 | 13 |
| `sitemap-blog-building.xml` | 搭建技巧 | 64 |
| `sitemap-blog-set-review.xml` | 积木评测 | 160 |
| `sitemap-blog-moc-review.xml` | MOC图纸评测 | 492 |
| `sitemap-blog-moc-tutorial.xml` | MOC教程 | 89 |
| `sitemap-blog-knowledge.xml` | 知识科普 | 2 |
| `sitemap-blog-resources.xml` | 资源推荐 | 17 |
| `sitemap-blog-other.xml` | 其他（历史未归类的旧分类） | 34 |
| **合计** | | **3550（2026-10-14 09:30 实测）** |

> 注：数字是快照。图纸详情页由内容生产流水线持续新增（09:13 快照 1922 → 09:30 实测 1975），各子 sitemap 会实时增长。本次拆分前后做了全量比对：**旧 sitemap 的 URL 一条没丢，新增的 83 条全是新增内容 + 30 个博客分页页**。

GSC 提交 `sitemap.xml` 即可，GSC 会自动展开子 sitemap 并分别给出「已提交/已编入索引」的数字。

---

## 二、站长操作详解

### 步骤 1：验证资源（二选一，推荐方式 A）

#### 方式 A：HTML 文件验证（推荐，服务器已就绪）

1. 打开 [Google Search Console](https://search.google.com/search-console)，点左上角资源下拉 → **添加资源**
2. 选左边 **「网址前缀」**，填 `https://brickplan.cn/`（注意带 https 和结尾斜杠）→ 继续
3. 验证方式选 **「HTML 文件」** → 会显示一个文件名，形如 `google1a2b3c4d5e6f7a8b.html` → **复制这个文件名**
4. 把文件名发到「积木社区运营开发AI工作群」（@开发02AI）。我们执行安装脚本后会回复「✅ GSC 验证文件就绪：…」
   - 有服务器权限的人也可以自己跑一条命令：
     `bash /home/ubuntu/.hermes/scripts/gsc_verify_setup.sh google1a2b3c4d5e6f7a8b.html`
5. 回到 GSC，点 **「验证」** → 应显示「已验证」。**不要下载 Google 给的 HTML 文件再上传**——本站没有文件上传入口，上传的是 dist 目录（会被前端构建清空，见「已知坑」）。

#### 方式 B：DNS TXT 验证（备选；想拿「网域」级数据时用）

「网域」属性（`brickplan.cn`，可覆盖 http/https、www、所有子域）**只能用 DNS 验证**。

1. GSC 添加资源时选 **「网域」**，填 `brickplan.cn` → 会给出一个 TXT 记录值，形如
   `google-site-verification=xxxxxxxxxxxxxxxxxxxxxxxxxxxx`
2. 登录 DNS 服务商控制台：本站 NS 是 `antony.dnspod.net` / `dylan.dnspod.net`，
   即 **腾讯云 DNSPod** → 域名 → `brickplan.cn` → 记录管理 → 添加记录：
   - 记录类型：`TXT`
   - 主机记录：`@`
   - 记录值：上面 GSC 给的完整值（`google-site-verification=…`，**整串照抄，不要加引号**）
   - 记录值长度超过 255 字符时，DNSPod 会自动分成多条 TXT，保持默认即可
3. 保存后等 5–30 分钟（DNSPod 默认 TTL 600s）。可先自检：
   `dig +short TXT brickplan.cn`
4. 回 GSC 点 **「验证」**

> 💡 建议：先按方式 A 拿到基本数据，后续想要跨子域/协议的完整视图，再按方式 B 补一个「网域」属性。两个属性互不影响。

### 步骤 2：提交 sitemap

1. GSC 左侧 **「站点地图」**
2. 「添加新的站点地图」输入 `sitemap.xml`（如果加的是网址前缀属性，填相对路径即可；也可直接填 `https://brickplan.cn/sitemap.xml`）→ 提交
3. 状态显示「成功」后，下面会列出 12 个子 sitemap 及各自 URL 数（对照上表）
4. `robots.txt` 里已经写了 `Sitemap: https://brickplan.cn/sitemap.xml`，无需额外操作

### 步骤 3：查看收录报告

- **总体**：GSC「索引 → 网页」→ 看「已编入索引 / 未编入索引」及原因分布（`已发现-尚未编入索引` 属正常，新站/新内容常见）
- **分主题**：GSC「站点地图」→ 点某个子 sitemap → 「已发现的网页」，即可看到该分类（如 MOC图纸评测 492 条）的收录情况
- **抓取情况**：GSC「设置 → 抓取统计信息」看 Googlebot 抓取量；本次拆分后每个文件都变小（最大 `sitemap-blueprints.xml` 382 KB → gzip 后 51 KB），对抓取更友好
- **人工检查**：GSC 顶部搜索框可直接输入任一 URL 做「网址检查」，看是否已被收录

---

## 三、已知坑 / 排错

| 现象 | 原因 | 处理 |
|---|---|---|
| GSC 报「验证失败：内容不正确」 | 返回的不是验证文件（例如被 SSR 兜底成 HTML 页） | `bash /home/ubuntu/.hermes/scripts/gsc_verify_setup.sh --check` 看 200 与内容；再确认 nginx location 存在 |
| `https://brickplan.cn/googleXXX.html` 返回 404 | 文件没放 / 文件名不一致 | 用脚本重放一次，注意**大小写与后缀 `.html`** 必须与 GSC 显示的一致 |
| GSC 报 sitemap「无法读取」 | 子 sitemap 返回 HTML 而非 XML | 逐个 `curl -I https://brickplan.cn/sitemap-blog-xxx.xml`，`Content-Type` 应为 `application/xml`，状态 200 |
| 验证文件突然失效 | 文件被放进了 `frontend/dist/`，被 `npm run build` 清掉 | **一律放 `/etc/nginx/seo-keys/`**（本操作单的目录），该目录不会被构建清理 |

### 顺带发现（本次体检，不在本任务范围，建议后续处理）

1. **百度/搜狗/字节 的站长验证文件全部 404**：`baidu_verify_codeva-xW5wPPXASH.html`、`sogousiteverification.txt`、`ByteDanceVerify.html` 在生产 dist 里已不存在（推测同样是前端构建清空 dist 所致）。建议从各站长平台重新下载验证文件，同样放进 `/etc/nginx/seo-keys/` 并改 location 的 root。
2. **4 篇文章因缺少 frontmatter 而 404**（dev/prod 都存在，2026-08-07 那批）：
   `gu-di-tui-jian.md`、`moc-ling-dong-chuang-xiang-ling-jian.md`、`moc-shu-dian-ji-mu-tao-zhuang.md`、`wan-ge-tui-jian.md`
   文件直接从 `# 标题` 开始，没有 `---title/date/category---` 头，Go 解析器要求 title → 整篇被丢弃（不出现在博客列表、不进 sitemap、URL 返回 404）。补上 frontmatter 即可，补完博客 URL 数会从 1533 → 1537。
