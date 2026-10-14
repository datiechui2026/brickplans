# Google Indexing API 批量提交 Runbook（brickplan.cn）

- 脚本：`/home/ubuntu/.hermes/scripts/brickplan_indexing_api.py`（已在生产实测 dry-run + JWT 自检）
- 状态：**脚本就绪，等 GSC 授权**。授权到位当天即可跑，不需要再改代码。
- 编写：开发02AI，2026-10-14

---

## 〇、先说清楚这条路的定位（重要，别当主力）

Google 官方文档写得很明确：**Indexing API 只支持 `JobPosting` 和 `BroadcastEvent`**。
2023 年 Google 打击滥用后，对其它内容类型**不保证处理**——即使返回 HTTP 200，也可能被忽略。

所以本次 1052 条「已发现-尚未编入索引」的**主力修复是内链 + sitemap + hreflang**（本轮已做）：
- 首页 /blog/ 链接 0 → 12 条（权重最高的已收录页现在给文章做引荐）
- 文章页相关推荐 3 → 5 条真实内链
- /blog 分页链完整（每页 rel=prev/next 真实 `<a>`），老文章可被逐页走完
- sitemap index + 语言×分类子文件全部 200，`xhtml:link` hreflang 就位

Indexing API 是**锦上添花**：用它把重点 URL 主动推一次，别指望它解决整体收录。
如果未来 Google 收紧到完全忽略，脚本可以直接停掉，不影响上面的修复。

---

## 一、授权前置（15 分钟，一次性的）

1. **GCP 建项目**：https://console.cloud.google.com/ → 新建项目 `brickplan-indexing`
2. **启用 API**：API 和服务 → 库 → 搜 `Indexing API` → 启用
3. **建服务账号**：IAM 和管理 → 服务账号 → 创建
   - 名称：`brickplan-indexing`
   - 不用授予项目级角色（GSC 侧授权即可）
4. **下 JSON 密钥**：该服务账号 → 密钥 → 添加密钥 → 创建新密钥 → JSON → 下载
   - 存到服务器（**不要进 git**）：`/home/ubuntu/.secrets/brickplan-indexing-sa.json`，`chmod 600`
5. **GSC 里授权这个 SA**（关键，少这步会 403）
   - Search Console → 设置 → 用户和权限 → 添加用户
   - 邮箱填服务账号的 `client_email`（形如 `brickplan-indexing@xxx.iam.gserviceaccount.com`）
   - 权限选 **「所有者」(Owner)** —— Indexing API 要求 owner 权限，用户级不够
6. 确认 GSC 已完成资源验证（见 `docs/gsc-onboarding.md`，卡在老板给验证文件名那一步）

---

## 二、跑法（授权后）

```bash
S=/home/ubuntu/.hermes/scripts/brickplan_indexing_api.py
SA=/home/ubuntu/.secrets/brickplan-indexing-sa.json

# 1) 先看计划（不需要凭据，不联网提交）
python3 $S --dry-run --limit 200

# 2) 只看英文树（M2 英文内容上线后优先推英文）
python3 $S --dry-run --lang en

# 3) 真提交：默认一次 200 条（对应默认日配额），自动分批（每批 ≤100）
python3 $S --credentials $SA --limit 200

# 4) 第二天继续（幂等，已提交的不会重复）
python3 $S --credentials $SA --limit 200

# 5) 查看进度
python3 $S --status
```

**配额**：默认 **200 URL/天/项目**（Google 可申请提额）。脚本按 `--limit` 控制单次用量，
配额用尽（403/429）会**立即停止并打印剩余量**，剩余留给下一次，不会把配额浪费在重试上。

**幂等/断点**：已成功的 URL 记录在 `~/.brickplan-indexing-state.json`，
重复运行自动跳过。要强制重提：`--resubmit`。

**URL 来源**：默认递归展开 `https://brickplan.cn/sitemap.xml`（已拆成 index + 语言×分类子文件，
直接 grep 根文件只会拿到子 sitemap 的 URL）。也可 `--urls /tmp/list.txt` 指定清单。
采集会自动跳过 `*.xml` 并去重。

---

## 三、验证脚本可用（不需要凭据，现在就能跑）

```bash
python3 /home/ubuntu/.hermes/scripts/brickplan_indexing_api.py --selftest
# → selftest OK: JWT(RS256) 签名与校验通过, claims = {...}

python3 /home/ubuntu/.hermes/scripts/brickplan_indexing_api.py --dry-run --limit 5
# → 采集 3654 条 | 本次计划 5 条 | 剩余 3649 条留给下次
```

`--selftest` 用临时生成的 RSA 私钥走完 `make_jwt()` → `pyjwt.decode()` 全链路，
证明签名/claims 拼接无误；不需要 GCP 凭据、不联外网。

---

## 四、排错

| 现象 | 原因 | 处理 |
|---|---|---|
| HTTP 403 `PERMISSION_DENIED` | GSC 里没把 SA 加成**所有者**，或资源未验证 | GSC 设置 → 用户和权限 → 加 `client_email` 为所有者 |
| HTTP 403 `Quota exceeded` / 429 | 当日 200 条配额用尽 | 明天再跑（脚本会自动接着上次的断点） |
| HTTP 401 | JSON 密钥失效 / 服务账号被删 | 重新下密钥 |
| `403 Indexing API has not been used in project...` | GCP 项目未启用 Indexing API | 步骤 1.2 启用 |
| 提交都 200 但收录不动 | 就是上面说的官方限制（非 JobPosting/BroadcastEvent 不保证处理） | 别加推，回到内链/sitemap 主线；用 GSC 的「网址检查 → 请求编入索引」做单条手动加速 |
| `ModuleNotFoundError: jwt` | PyJWT 缺失 | `pip3 install pyjwt cryptography`（本机已有 pyjwt 2.7.0 / cryptography 41.0.7） |

---

## 五、其它引擎

- **Bing**：不用 Indexing API，走 IndexNow（cron `742723f53ba9`，每天 09:00/21:00，全量 URL 含英文）
- **百度**：只推中文 URL；配额 ~10-20/日（cron 已存在）
- **收录监控**：cron `13bf885fb8a5` 盯外链落地页，后续扩展英文 URL
