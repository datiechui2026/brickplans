#!/usr/bin/env python3
"""Rewrite the Chinese blog corpus into English, one file per slug.

Why a rewrite and not a translation
-----------------------------------
The English tree (content/en/blog) is indexed as its own language version of the
same URLs (hreflang pairs are emitted only when the same slug exists in both
trees). Google treats a mechanical translation of 1,500 pages as scaled content
with no added value; articles written FOR an English reader (search intent,
examples, units, brand names) survive that test. So the prompt asks for a
rewrite that preserves facts/numbers/part names and section structure, not a
sentence-by-sentence translation.

Output contract (matches content/en/blog/*.md shipped so far)
-------------------------------------------------------------
  ---
  title: "..."
  description: "..."
  date: <source date>
  author: "BrickPlan Team"
  category: "<one of the EN category labels>"
  tags: [tag one, tag two]
  ---
  # <English title>
  ...body...

The slug is the filename and MUST equal the Chinese slug, otherwise the hreflang
pair (blog.HasLang) and the sitemap grouping never see the pair.

Usage
-----
  python scripts/en_rewrite.py --stats
  python scripts/en_rewrite.py --limit 5            # next 5, resumable
  python scripts/en_rewrite.py --slug foo --force   # regenerate one
  python scripts/en_rewrite.py --limit 100 --concurrency 6

Requires ANTHROPIC_API_KEY (and optionally ANTHROPIC_BASE_URL /
ANTHROPIC_MODEL, which this environment points at an Anthropic-compatible
endpoint). Existing English files are skipped, so the job is cheap to resume.
"""

import argparse
import concurrent.futures
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
ZH_DIR = REPO / "backend-go" / "content" / "blog"
EN_DIR = REPO / "backend-go" / "content" / "en" / "blog"

API_URL = os.environ.get("ANTHROPIC_BASE_URL", "https://api.anthropic.com").rstrip("/") + "/v1/messages"
API_KEY = os.environ.get("ANTHROPIC_API_KEY", "")
MODEL = os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-5-5")

# ── Category mapping ────────────────────────────────────────────────────────
# Mirrors backend-go/internal/blog/categories.go. The bucket decides the EN
# category label (and therefore the sitemap bucket); unknown values must not
# drop the page, so they fall through to the catch-all.
BUCKET_LABEL_EN = {
    "brand": "Brand Guide",
    "beginner": "Beginner Guide",
    "parts": "Parts Guide",
    "building": "Building Techniques",
    "set-review": "Set Review",
    "moc-review": "MOC Review",
    "moc-tutorial": "MOC Tutorial",
    "knowledge": "Knowledge",
    "resources": "Resources",
    "other": "Knowledge",
}

RAW_TO_BUCKET = {}
for _slug, _raws in {
    "brand": ["品牌推荐", "品牌评测", "品牌选购", "国产积木", "积木推荐", "选购指南",
              "Brand Guide", "Brand Review", "Buying Guide", "Brand Comparison",
              "Brand", "Brands", "Brand Ranking", "Brand Overview"],
    "beginner": ["新手教程", "新手入门", "新手指南", "积木教程", "搭建教程", "教程",
                 "拼装教程", "拼装指南", "实用指南", "指南", "亲子玩法",
                 "Beginner Guide", "Getting Started", "Tutorial", "How To"],
    "parts": ["零件知识", "零件指南", "积木零件", "Parts Guide", "Brick Parts", "Parts Explained"],
    "building": ["搭建技巧", "拼装技巧", "收纳技巧", "积木搭建", "MOC搭建", "场景MOC",
                 "场景搭建", "Building Tips", "Techniques", "Storage Tips",
                 "Building Techniques", "Building Technique", "Building Skills"],
    "set-review": ["积木评测", "套装评测", "积木套装评测", "系列评测", "套装推荐", "评测",
                   "Set Review", "Set Reviews", "Reviews"],
    "moc-review": ["MOC图纸评测", "图纸评测", "MOC图纸", "MOC评测",
                   "MOC Review", "Blueprint Review", "Instructions Review"],
    "moc-tutorial": ["MOC教程", "MOC 教程", "MOC入门", "MOC指南", "图纸指南", "图纸教程",
                     "设计教程", "MOC场景指南", "MOC Tutorial", "MOC Guide",
                     "Building Guide", "Instructions Guide"],
    "knowledge": ["知识科普", "MOC知识", "Knowledge", "Explained"],
    "resources": ["资源推荐", "图纸下载", "MOC分享", "找图指南", "推荐",
                  "Resources", "Downloads", "Where To Find"],
}.items():
    for _raw in _raws:
        RAW_TO_BUCKET[_raw] = _slug
# Case-insensitive fallback, same rule as blog.normToBucket (no whitespace trim).
NORM_TO_BUCKET = {k.lower(): v for k, v in RAW_TO_BUCKET.items()}


def category_label(raw):
    if raw in RAW_TO_BUCKET:
        return BUCKET_LABEL_EN[RAW_TO_BUCKET[raw]]
    return BUCKET_LABEL_EN[NORM_TO_BUCKET.get(raw.lower(), "other")]


# ── Frontmatter ─────────────────────────────────────────────────────────────
def split_frontmatter(text):
    m = re.match(r"^---\s*\n(.*?)\n---\s*\n?(.*)$", text, re.S)
    if not m:
        return {}, text
    meta = {}
    for line in m.group(1).splitlines():
        km = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if km:
            meta[km.group(1)] = km.group(2).strip()
    return meta, m.group(2)


def unquote(v):
    v = v.strip()
    if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
        return v[1:-1]
    return v


def parse_tags(v):
    v = v.strip().strip("[]")
    return [unquote(t) for t in v.split(",") if t.strip()]


def yaml_scalar(v):
    return '"' + v.replace("\\", "\\\\").replace('"', '\\"') + '"'


# ── Validation ──────────────────────────────────────────────────────────────
CJK = re.compile(r"[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]")
IMG = re.compile(r"!\[[^\]]*\]\((https?://[^)\s]+)\)")


def validate(result, src):
    problems = []
    title = (result.get("title") or "").strip()
    desc = (result.get("description") or "").strip()
    body = (result.get("body") or "").strip()
    tags = result.get("tags") or []

    if not title:
        problems.append("missing title")
    elif CJK.search(title):
        problems.append("title contains CJK")
    elif len(title) > 75:
        problems.append(f"title too long ({len(title)})")

    if not desc:
        problems.append("missing description")
    elif CJK.search(desc):
        problems.append("description contains CJK")
    elif not 60 <= len(desc) <= 180:
        problems.append(f"description length {len(desc)} outside 60-180")

    if CJK.search(body):
        problems.append("body contains CJK characters")
    if len(body.split()) < 400:
        problems.append(f"body too short ({len(body.split())} words)")
    if not body.startswith("# "):
        problems.append("body must start with the H1")

    src_imgs = IMG.findall(src)
    out_imgs = IMG.findall(body)
    missing = [u for u in src_imgs if u not in out_imgs]
    if missing:
        problems.append(f"{len(missing)} source image(s) dropped")

    if not isinstance(tags, list) or not 2 <= len(tags) <= 8:
        problems.append("need 2-8 tags")
    elif any(not str(t).strip() or CJK.search(str(t)) for t in tags):
        problems.append("tags must be non-empty English strings")

    return problems


# ── Prompt ──────────────────────────────────────────────────────────────────
SYSTEM = """You are a senior English editor for BrickPlan, a community site about brick building and MOC (My Own Creation) models. You adapt Chinese brick articles for English-speaking builders.

Rules:
- REWRITE for an English reader; do not translate sentence by sentence. Keep every factual claim, number, brick part name, part number, dimension, price range, difficulty level and step order from the source. Never invent facts, products, prices or part numbers.
- Keep the same section structure (same number and order of H2/H3 sections). Keep markdown tables and lists.
- Keep every image from the source EXACTLY as-is (same URL); translate only the alt text.
- Rewrite internal links: /blog/<slug> -> /en/blog/<slug>, /tags/<name> -> /en/tags/<name>. Leave /detail/<id> unchanged.
- Use standard English names for brick brands: 乐高=LEGO, 森宝=SEMBO, 双鹰=Double Eagle, 宇星模王=Mould King, 高德斯=Gobricks, 启蒙=Qiji, 布鲁可=Bloks, 咔搭=Cada, 小米积木=Xiaomi, 万格=Wange, 星堡=StarBricks, 开智=Kazi. Chinese part names use LEGO terminology (2x4 brick, plate, tile, slope, technic pin).
- Output 800-1400 words unless the source is clearly shorter. Do not pad with filler, do not repeat the title as a paragraph, no "In conclusion" fluff.
- No Chinese characters anywhere in the output (not even in parentheses). Brand names, set numbers and part numbers stay in Latin characters/digits.
- The body must start with "# <English title>" and contain only the article markdown (no frontmatter).
- Title: specific, search-intent oriented, <= 65 characters, no clickbait, no brand-stuffing.
- Description: 120-160 characters, states what the reader will learn.
- Tags: 3-6 short lowercase English search phrases.

Return ONLY valid JSON:
{"title": "...", "description": "...", "tags": ["..."], "body": "..."}"""


def build_user_prompt(slug, meta, body, category):
    return (
        f"Slug: {slug}\n"
        f"English category (fixed, do not change): {category}\n"
        f"Source title: {unquote(meta.get('title', ''))}\n"
        f"Source description: {unquote(meta.get('description', ''))}\n"
        f"Source tags: {', '.join(parse_tags(meta.get('tags', '')))}\n\n"
        f"Source article markdown:\n\n{body}"
    )


def call_model(prompt, max_tokens=8000):
    payload = {
        "model": MODEL,
        "max_tokens": max_tokens,
        "system": SYSTEM,
        "messages": [{"role": "user", "content": prompt}],
    }
    req = urllib.request.Request(
        API_URL,
        data=json.dumps(payload).encode(),
        headers={
            "content-type": "application/json",
            "x-api-key": API_KEY,
            "anthropic-version": "2023-06-01",
        },
        method="POST",
    )
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=240) as resp:
                data = json.loads(resp.read())
            text = "".join(b.get("text", "") for b in data.get("content", []) if b.get("type") == "text")
            usage = data.get("usage", {})
            return text, usage
        except urllib.error.HTTPError as e:
            if e.code in (429, 500, 502, 503, 529) and attempt < 3:
                time.sleep(5 * (attempt + 1))
                continue
            raise RuntimeError(f"HTTP {e.code}: {e.read()[:300]!r}")
        except (urllib.error.URLError, TimeoutError) as e:
            if attempt < 3:
                time.sleep(5 * (attempt + 1))
                continue
            raise RuntimeError(f"network: {e}")
    raise RuntimeError("unreachable")


def extract_json(text):
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```[a-zA-Z]*\s*", "", text)
        text = re.sub(r"\s*```$", "", text)
    start, end = text.find("{"), text.rfind("}")
    if start < 0 or end < 0:
        raise ValueError("no JSON object in response")
    return json.loads(text[start:end + 1])


def rewrite_internal_links(body):
    body = re.sub(r"\]\(/blog/([^)\s#?]+)", r"](/en/blog/\1", body)
    body = re.sub(r"\]\(/tags/([^)\s#?]+)", r"](/en/tags/\1", body)
    return body


# ── File IO ─────────────────────────────────────────────────────────────────
def render_file(slug_title, meta, result):
    tags = ", ".join(str(t).strip() for t in result["tags"])
    return (
        "---\n"
        f"title: {yaml_scalar(result['title'].strip())}\n"
        f"description: {yaml_scalar(result['description'].strip())}\n"
        f"date: {meta.get('date', '').strip()}\n"
        'author: "BrickPlan Team"\n'
        f"category: {yaml_scalar(category_label(unquote(meta.get('category', ''))))}\n"
        f"tags: [{tags}]\n"
        "---\n\n"
        f"{rewrite_internal_links(result['body'].strip())}\n"
    )


def load_sources():
    out = []
    for p in sorted(ZH_DIR.glob("*.md")):
        out.append((p.stem, p))
    return out


def priority_key(item):
    """Evergreen search-intent categories first, then newest first."""
    slug, path = item
    meta, _ = split_frontmatter(path.read_text(encoding="utf-8", errors="replace"))
    bucket = RAW_TO_BUCKET.get(unquote(meta.get("category", "")), "other")
    order = {"beginner": 0, "parts": 1, "building": 2, "moc-tutorial": 3,
             "brand": 4, "set-review": 5, "moc-review": 6, "knowledge": 7,
             "resources": 8, "other": 9}
    return (order.get(bucket, 9), _neg_date(meta.get("date", "")), slug)


def _neg_date(d):
    # Newest first: reverse lexicographic on ISO-ish dates.
    d = d.strip().strip('"')
    return "".join(chr(255 - ord(c)) for c in d)


EN_LABELS = set(BUCKET_LABEL_EN.values())


def verify_all(sources):
    """Static checks over the whole EN tree (no API calls). Returns #problems."""
    zh = dict(sources)
    problems = 0
    files = sorted(EN_DIR.glob("*.md"))
    for p in files:
        slug = p.stem
        text = p.read_text(encoding="utf-8", errors="replace")
        meta, body = split_frontmatter(text)
        errs = []
        if slug not in zh:
            errs.append("no Chinese source")
        else:
            _, src_body = split_frontmatter(zh[slug].read_text(encoding="utf-8", errors="replace"))
            # The original 18 articles ship dedicated en-blog/ illustrations, so
            # compare COUNTS; the rewriter is instructed to keep URLs verbatim.
            if len(IMG.findall(body)) < len(IMG.findall(src_body)):
                errs.append(f"{len(IMG.findall(src_body)) - len(IMG.findall(body))} source image(s) dropped")
        if CJK.search(text):
            errs.append("contains CJK characters")
        if unquote(meta.get("category", "")) not in EN_LABELS:
            errs.append(f"unknown category {meta.get('category')!r}")
        for k in ("title", "description", "date", "author", "category", "tags"):
            if not meta.get(k):
                errs.append(f"missing {k}")
        if len(body.split()) < 400:
            errs.append(f"body too short ({len(body.split())} words)")
        if not body.lstrip().startswith("# "):
            errs.append("body does not start with H1")
        if errs:
            problems += 1
            print(f"{slug}: {'; '.join(errs)}")
    print(f"checked {len(files)} EN file(s), {problems} problem(s)")
    return problems


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=0, help="rewrite at most N missing articles")
    ap.add_argument("--slug", action="append", default=[], help="rewrite a specific slug")
    ap.add_argument("--concurrency", type=int, default=4)
    ap.add_argument("--force", action="store_true", help="overwrite existing EN files")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--stats", action="store_true")
    ap.add_argument("--verify", action="store_true", help="check existing EN files without calling the API")
    args = ap.parse_args()

    sources = load_sources()
    zh_slugs = {s for s, _ in sources}
    en_slugs = {p.stem for p in EN_DIR.glob("*.md")} if EN_DIR.exists() else set()

    if args.stats:
        print(f"zh posts: {len(sources)}")
        print(f"en posts: {len(en_slugs)}")
        print(f"shared slugs: {len(zh_slugs & en_slugs)}")
        print(f"missing: {len(zh_slugs - en_slugs)}")
        return

    if args.verify:
        sys.exit(1 if verify_all(sources) else 0)

    if not API_KEY:
        sys.exit("ANTHROPIC_API_KEY is not set")

    EN_DIR.mkdir(parents=True, exist_ok=True)
    by_slug = dict(sources)
    if args.slug:
        todo = [(s, by_slug[s]) for s in args.slug if s in by_slug and (args.force or s not in en_slugs)]
    else:
        todo = sorted([(s, p) for s, p in sources if args.force or s not in en_slugs],
                      key=priority_key)
        if args.limit:
            todo = todo[:args.limit]

    print(f"{len(todo)} article(s) to rewrite; model={MODEL}")
    if args.dry_run:
        for s, p in todo:
            print(" ", s, p)
        return

    totals = {"in": 0, "out": 0, "ok": 0, "failed": []}
    lock = __import__("threading").Lock()

    def work(item):
        slug, path = item
        src = path.read_text(encoding="utf-8", errors="replace")
        meta, body = split_frontmatter(src)
        if not meta.get("date", "").strip():
            # A handful of legacy posts carry no frontmatter at all. Use the
            # source file's mtime so the English page still has a publication
            # date (the Go parser would otherwise emit 0001-01-01 into lastmod).
            meta["date"] = time.strftime("%Y-%m-%d", time.localtime(path.stat().st_mtime))
        category = category_label(unquote(meta.get("category", "")))
        prompt = build_user_prompt(slug, meta, body, category)

        last_problems = []
        for attempt in range(2):
            text, usage = call_model(prompt if attempt == 0 else
                                     prompt + f"\n\nPrevious attempt failed validation: {'; '.join(last_problems)}. Fix and return JSON only.")
            try:
                result = extract_json(text)
            except (ValueError, json.JSONDecodeError) as e:
                last_problems = [f"invalid JSON ({e})"]
                continue
            last_problems = validate(result, src)
            if not last_problems:
                out = render_file(slug, meta, result)
                tmp = EN_DIR / f".{slug}.tmp"
                # newline="\n": the Go parser splits frontmatter on "\n---";
                # Python's default CRLF translation on Windows would make every
                # generated post unparseable (silently skipped, title empty).
                tmp.write_text(out, encoding="utf-8", newline="\n")
                tmp.replace(EN_DIR / f"{slug}.md")
                with lock:
                    totals["in"] += usage.get("input_tokens", 0)
                    totals["out"] += usage.get("output_tokens", 0)
                    totals["ok"] += 1
                return f"ok   {slug} ({len(result['body'].split())} words, cat={category_label(unquote(meta.get('category','')))})"
        with lock:
            totals["failed"].append(slug)
        return f"FAIL {slug}: {'; '.join(last_problems)}"

    start = time.time()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as ex:
        futures = {ex.submit(work, item): item[0] for item in todo}
        for i, fut in enumerate(concurrent.futures.as_completed(futures), 1):
            try:
                print(f"[{i}/{len(todo)}] {fut.result()}", flush=True)
            except Exception as e:  # keep the batch alive
                slug = futures[fut]
                with lock:
                    totals["failed"].append(slug)
                print(f"[{i}/{len(todo)}] FAIL {slug}: {e}", flush=True)

    mins = (time.time() - start) / 60
    print(f"\ndone: {totals['ok']} ok, {len(totals['failed'])} failed in {mins:.1f} min")
    print(f"tokens: {totals['in']} in / {totals['out']} out")
    if totals["failed"]:
        print("failed slugs:", " ".join(totals["failed"]))
        sys.exit(1)


if __name__ == "__main__":
    main()
