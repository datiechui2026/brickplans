#!/usr/bin/env python3
"""brickplan_indexing_api.py — Google Indexing API 批量提交（brickplan.cn）

用途：把 sitemap 里的 URL 分批提交给 Google Indexing API，加速收录。
设计目标：**授权到位的当天就能直接跑，不需要再写代码**。

⚠️ 官方限制（必须先读）
  Google 文档明确 Indexing API 支持的 only JobPosting / BroadcastEvent。
  2023 起因滥用，Google 对其它内容类型**不保证处理**（提交返回 200 也可能被忽略）。
  所以本脚本定位是「锦上添花」：主力仍然是内链 + sitemap + hreflang。
  别把它当成 1052 条的解法——那批要靠首页/正文内链把referrer补上。

用法
  # 1) 先看计划（无需凭据，不提交）
  python3 brickplan_indexing_api.py --dry-run
  # 2) 自检 JWT 签名链路（临时生成密钥，不联网）
  python3 brickplan_indexing_api.py --selftest
  # 3) 真提交（拿到 service account JSON 之后）
  python3 brickplan_indexing_api.py --credentials /path/sa.json --limit 200
  python3 brickplan_indexing_api.py --credentials /path/sa.json --urls /tmp/urls.txt
  # 4) 只提交英文树 / 只看状态
  python3 brickplan_indexing_api.py --credentials sa.json --lang en --limit 50
  python3 brickplan_indexing_api.py --status

幂等与断点
  已提交成功的 URL 记在 --state（默认 /home/ubuntu/.brickplan-indexing-state.json），
  重复运行不会重复提交；用 --resubmit 强制重提。
  配额用尽（HTTP 403 / 429）时立即停止并把剩余 URL 留给下一次，不会把配额浪费在重试上。
"""

import argparse
import gzip
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

SITE = "https://brickplan.cn"
SITEMAP = SITE + "/sitemap.xml"
DEFAULT_STATE = os.path.expanduser("~/.brickplan-indexing-state.json")
TOKEN_URL = "https://oauth2.googleapis.com/token"
SCOPE = "https://www.googleapis.com/auth/indexing"
BATCH_ENDPOINT = "https://indexing.googleapis.com/batch"  # 每批最多 100 条
MAX_BATCH = 100


# ── URL 采集 ──────────────────────────────────────────────────────────────

def fetch(url: str, timeout: int = 30) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": "brickplan-indexing/1.0"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read()
    if raw[:2] == b"\x1f\x8b":
        raw = gzip.decompress(raw)
    return raw


def collect_urls(sitemap: str = SITEMAP, lang: str = "", depth: int = 0) -> list:
    """递归展开 sitemap index。sitemap 已拆成 index + 语言×分类子文件，
    直接 grep 根文件只会拿到子 sitemap 的 URL。"""
    if depth > 3:
        return []
    try:
        root = ET.fromstring(fetch(sitemap))
    except Exception as exc:  # noqa: BLE001
        print(f"WARN: 无法读取 {sitemap}: {exc}", file=sys.stderr)
        return []
    ns = "{http://www.sitemaps.org/schemas/sitemap/0.9}"
    if root.tag.endswith("sitemapindex"):
        out = []
        for loc in root.iter(ns + "loc"):
            out += collect_urls(loc.text.strip(), lang, depth + 1)
        return out
    urls = [loc.text.strip() for loc in root.iter(ns + "loc") if loc.text]
    if lang == "en":
        return [u for u in urls if "/en/blog" in u]
    if lang == "zh":
        return [u for u in urls if "/en/blog" not in u]
    return urls


def dedupe(urls: list) -> list:
    seen, out = set(), []
    for u in urls:
        if u and u not in seen and not u.endswith(".xml"):
            seen.add(u)
            out.append(u)
    return out


# ── JWT / OAuth（RS256，service account）──────────────────────────────────

def load_sa(cred_path):
    """凭据来源优先级：
      1) --credentials <path>
      2) env GOOGLE_SA_JSON  —— JSON 原文，或 base64（GitHub Actions secret 用这个）
      3) env GOOGLE_SA_JSON_FILE —— 指向一个 JSON 文件
    不写盘、不进仓库，适配 CI。"""
    if cred_path:
        return json.load(open(cred_path, encoding="utf-8"))
    raw = (os.environ.get("GOOGLE_SA_JSON") or "").strip()
    if raw:
        if not raw.lstrip().startswith("{"):
            import base64
            raw = base64.b64decode(raw).decode("utf-8")
        return json.loads(raw)
    p = (os.environ.get("GOOGLE_SA_JSON_FILE") or "").strip()
    if p:
        return json.load(open(p, encoding="utf-8"))
    return None


def parse_batch_parts(body, n):
    """⚠️ 关键：Google batch 接口的**整体 HTTP 状态恒为 200**，逐条结果只在 multipart body 里。
    只看整体状态会把「未授权 → 逐条 403 PERMISSION_DENIED」误判成提交成功。
    返回按请求顺序排列的逐条状态码。"""
    codes = [int(m.group(1)) for m in re.finditer(r"HTTP/1\.1\s+(\d{3})", body)]
    return codes


def first_error(body, limit=400):
    m = re.search(r'"message"\s*:\s*"([^"]{0,%d})"' % limit, body)
    if m:
        return m.group(1)[:limit]
    m = re.search(r"(PERMISSION_DENIED|SERVICE_DISABLED|ACCESS_TOKEN_SCOPE_INSUFFICIENT|quota)", body, re.I)
    return m.group(1) if m else ""


def make_jwt(sa: dict, ttl: int = 3600) -> str:
    import jwt  # PyJWT
    now = int(time.time())
    claims = {
        "iss": sa["client_email"],
        "scope": SCOPE,
        "aud": TOKEN_URL,
        "iat": now,
        "exp": now + ttl,
    }
    return jwt.encode(claims, sa["private_key"], algorithm="RS256")


def access_token(sa: dict) -> str:
    assertion = make_jwt(sa)
    body = urllib.parse.urlencode({
        "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
        "assertion": assertion,
    }).encode()
    req = urllib.request.Request(TOKEN_URL, data=body, headers={"Content-Type": "application/x-www-form-urlencoded"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())["access_token"]


# ── 提交 ──────────────────────────────────────────────────────────────────

def submit_batch(token: str, urls: list, action: str = "URL_UPDATED") -> dict:
    """一个 multipart batch 请求提交最多 100 条。返回 {url: (status, body)}。"""
    boundary = "brickplan-batch-%d" % int(time.time() * 1000)
    parts = []
    for i, u in enumerate(urls):
        parts.append(
            f"--{boundary}\r\n"
            "Content-Type: application/http\r\n"
            "Content-Transfer-Encoding: binary\r\n"
            f"Content-ID: <{i+1}>\r\n\r\n"
            "POST /v3/urlNotifications:publish HTTP/1.1\r\n"
            "Content-Type: application/json\r\n\r\n"
            + json.dumps({"url": u, "type": action})
            + "\r\n"
        )
    body = ("".join(parts) + f"--{boundary}--\r\n").encode()
    req = urllib.request.Request(
        BATCH_ENDPOINT,
        data=body,
        headers={
            "Authorization": "Bearer " + token,
            "Content-Type": "multipart/mixed; boundary=" + boundary,
        },
    )
    with urllib.request.urlopen(req, timeout=90) as resp:
        return {"status": resp.status, "body": resp.read().decode("utf-8", "replace")}


def load_state(path: str) -> dict:
    if os.path.exists(path):
        try:
            return json.load(open(path, encoding="utf-8"))
        except Exception:  # noqa: BLE001
            return {}
    return {}


def save_state(path: str, state: dict) -> None:
    tmp = path + ".tmp"
    json.dump(state, open(tmp, "w", encoding="utf-8"), ensure_ascii=False, indent=2, sort_keys=True)
    os.replace(tmp, path)


# ── 自检 / 干跑 ───────────────────────────────────────────────────────────

def selftest() -> int:
    """用临时生成的 RSA 密钥走一遍 make_jwt，证明签名链路可用（不需要 Google 凭据）。"""
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import rsa
    import jwt as pyjwt
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    ).decode()
    sa = {"client_email": "selftest@example.iam.gserviceaccount.com", "private_key": pem}
    tok = make_jwt(sa)
    pub = key.public_key().public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    )
    decoded = pyjwt.decode(tok, pub, algorithms=["RS256"], audience=TOKEN_URL)
    assert decoded["iss"] == sa["client_email"] and decoded["scope"] == SCOPE
    print("selftest OK: JWT(RS256) 签名与校验通过，claims =", {k: decoded[k] for k in ("iss", "aud", "scope")})
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description="brickplan.cn Google Indexing API 批量提交")
    ap.add_argument("--credentials", help="service account JSON 路径")
    ap.add_argument("--urls", help="URL 列表文件（默认从 sitemap 递归采集）")
    ap.add_argument("--lang", choices=["", "zh", "en"], default="", help="只提交某个语言树")
    ap.add_argument("--limit", type=int, default=200, help="本次最多提交多少条（默认 200，对应默认日配额）")
    ap.add_argument("--batch", type=int, default=MAX_BATCH, help="每批条数，上限 100")
    ap.add_argument("--state", default=DEFAULT_STATE, help="已提交记录（幂等/断点）")
    ap.add_argument("--action", default="URL_UPDATED", choices=["URL_UPDATED", "URL_DELETED"])
    ap.add_argument("--dry-run", action="store_true", help="只打印计划，不提交、不需要凭据")
    ap.add_argument("--status", action="store_true", help="打印已提交统计")
    ap.add_argument("--resubmit", action="store_true", help="忽略去重记录，强制重提")
    ap.add_argument("--selftest", action="store_true", help="自检 JWT 签名链路")
    ap.add_argument("--json-out", default="", help="把运行摘要写成 JSON（CI 用，便于判定真假成功）")
    args = ap.parse_args()

    if args.selftest:
        return selftest()

    state = load_state(args.state)
    if args.status:
        done = state.get("submitted", {})
        print(f"已提交 {len(done)} 条 | state 文件: {args.state}")
        for lang in ("zh", "en"):
            print(f"  {lang}: {sum(1 for u in done if ('/en/blog' in u) == (lang == 'en'))}")
        if done:
            last = sorted(done.items(), key=lambda kv: kv[1])[-1]
            print("  最近一条:", last[0], last[1])
        return 0

    urls = []
    if args.urls:
        urls = [l.strip() for l in open(args.urls, encoding="utf-8") if l.strip()]
        print(f"URL 来源: {args.urls}")
    else:
        urls = collect_urls(lang=args.lang)
        print(f"URL 来源: {SITEMAP}（递归展开，lang={args.lang or 'all'}）")
    urls = dedupe(urls)

    done = state.setdefault("submitted", {})
    pending = urls if args.resubmit else [u for u in urls if u not in done]
    todo = pending[: args.limit]

    print(f"采集 {len(urls)} 条 | 已提交过 {len(urls) - len(pending)} 条 | 本次计划 {len(todo)} 条 | 剩余 {max(0, len(pending) - len(todo))} 条留给下次")
    if args.dry_run:
        for u in todo[:10]:
            print("   ", u)
        if len(todo) > 10:
            print(f"    …（还有 {len(todo) - 10} 条）")
        print("dry-run：未提交任何请求")
        return 0
    if not todo:
        print("没有需要提交的 URL")
        return 0
    try:
        sa = load_sa(args.credentials)
    except Exception as exc:  # noqa: BLE001
        print("ERROR: 无法载入 service account 凭据: %s" % exc, file=sys.stderr)
        return 2
    if not sa:
        print("ERROR: 缺凭据 → 给 --credentials <sa.json>，或设 GOOGLE_SA_JSON"
              "（JSON 原文 / base64，GitHub Actions 用这个）", file=sys.stderr)
        return 2

    try:
        token = access_token(sa)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", "replace")[:300]
        print("ERROR: 获取 access token 失败 HTTP %s: %s" % (exc.code, detail), file=sys.stderr)
        if "invalid_grant" in detail:
            print("提示: SA 私钥无效/已撤销，或系统时间偏差过大。", file=sys.stderr)
        return 3
    print("已获取 access token（service account: %s）" % sa.get("client_email"))

    batch = min(args.batch, MAX_BATCH)
    ok = fail = 0
    perm_denied = False
    codes_all = {}
    t_start = time.time()
    for i in range(0, len(todo), batch):
        chunk = todo[i:i + batch]
        try:
            res = submit_batch(token, chunk, args.action)
        except urllib.error.HTTPError as exc:
            code = exc.code
            detail = exc.read().decode("utf-8", "replace")[:300]
            print("ERROR: batch HTTP %s: %s" % (code, detail), file=sys.stderr)
            if code in (401, 403, 429):
                print("STOP: HTTP %s（权限/配额）→ 已提交 %d 条，剩余留给下次。" % (code, ok), file=sys.stderr)
                fail += len(chunk)
                break
            fail += len(chunk)
            continue

        # ⚠️ 整体 200 不代表逐条成功：必须解析 multipart body 的逐条状态
        codes = parse_batch_parts(res["body"], len(chunk))
        for c in codes:
            codes_all[c] = codes_all.get(c, 0) + 1
        if not codes:
            # 兜底：body 不是 multipart（例如整个请求被拒），按整体状态算
            print("WARN: 响应体无逐条状态，body 前 200 字: %s" % res["body"][:200], file=sys.stderr)
            fail += len(chunk)
            continue

        n_ok = 0
        for u, c in zip(chunk, codes):
            if 200 <= c < 300:
                done[u] = time.strftime("%Y-%m-%dT%H:%M:%S")
                n_ok += 1
        ok += n_ok
        fail += len(chunk) - n_ok
        save_state(args.state, state)
        print("  批 %d: %d 条 → 逐条 2xx %d / 非 2xx %d | 状态分布 %s"
              % (i // batch + 1, len(chunk), n_ok, len(chunk) - n_ok,
                 {k: v for k, v in sorted(codes_all.items())}))

        if any(c == 403 for c in codes):
            perm_denied = True
            print("\nSTOP: 逐条返回 403 PERMISSION_DENIED → **GSC 授权未完成**。", file=sys.stderr)
            print("      需要站点所有者在 Search Console 里把 SA 加为「所有者」：", file=sys.stderr)
            print("      %s" % sa.get("client_email"), file=sys.stderr)
            print("      详情: %s" % first_error(res["body"]), file=sys.stderr)
            break
        time.sleep(1)  # 温和节流

    save_state(args.state, state)
    dur = time.time() - t_start
    summary = {
        "site": SITE, "sa": sa.get("client_email"), "planned": len(todo),
        "ok": ok, "fail": fail, "http_codes": codes_all,
        "permission_denied": perm_denied, "seconds": round(dur, 1),
        "state_file": args.state, "state_total": len(state.get("submitted", {})),
    }
    print("\n完成：成功 %d 条，失败 %d 条，耗时 %.0fs，state 累计 %d 条 → %s"
          % (ok, fail, dur, summary["state_total"], args.state))
    if args.json_out:
        json.dump(summary, open(args.json_out, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
        print("[json] %s" % args.json_out)
    if perm_denied:
        print("结论: ❌ GSC 未授权（403 PERMISSION_DENIED）—— 非代码问题，需老板在 GSC 加 SA 为所有者。")
        return 3
    if ok and fail == 0:
        print("结论: ✅ 全部 %d 条 Google 真实返回 2xx" % ok)
        return 0
    # 429 是配额限流（Indexing API 每日/每分钟都有硬配额），不是故障：
    # 剩余 URL 已留在 state 里下次继续，没必要把整条流水线变红。
    nonok = [c for c in codes_all if not (200 <= c < 300)]
    if fail and nonok and all(c == 429 for c in nonok):
        print("结论: ⏳ 命中配额限流（429）—— 已提交 %d 条，剩余留给下次（state 已保存，非故障）" % ok)
        return 0
    print("结论: ⚠️ 部分失败（成功 %d / 失败 %d，状态分布 %s）" % (ok, fail, codes_all))
    return 1


if __name__ == "__main__":
    import urllib.parse  # noqa: E402  (access_token 用)
    sys.exit(main())
