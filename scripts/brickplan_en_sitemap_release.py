#!/usr/bin/env python3
"""Release the next batch of English articles into the sitemap exposure ledger.

Why this exists
---------------
1,536 English translations landed in one batch. Advertising all of them in
`sitemap-en-blog-*.xml` on the same day is the shape Google's scaled-content
policy looks for, so `internal/blog/exposure.go` gates the English article
sitemaps behind `content/en/sitemap-exposure.txt` and releases 100 slugs a day.
This script is the thing that actually appends those 100 — without it the gate
freezes at whatever the seed file contained (155 on 2026-10-24) and the rest of
the corpus is never advertised.

What "release" means here: the slug starts appearing in `sitemap-en-blog-*.xml`,
loses its `noindex` / `X-Robots-Tag: noindex, follow`, appears in the English
listing again and is offered as an `hreflang` alternate — all four read the same
ledger, and the backend re-stats it per request, so no restart is needed.

Design notes
------------
* Deterministic order: judgement-day order is alphabetical by slug, matching the
  seed file, so a batch is reproducible from the ledger alone.
* Idempotent per day: the header records the last release as
  `# last release 2026-10-24T07:46:51Z: +100 slugs (total 155)`. If today (UTC)
  is already in there, the run is a no-op unless --force. That makes an hourly
  or boot-time timer safe.
* Atomic: the new file is written to a sibling temp file, fsynced and
  `os.replace`d, so a crawler never reads a half-written ledger and a crash
  cannot truncate the rollout (the backend also keeps its last good view on a
  read error — see ExposureLedger.refresh).
* Rollback: delete the ledger file → the gate turns itself off and every
  English article is advertised again. `--status` prints how far the rollout has
  got.

Usage
-----
    python3 scripts/brickplan_en_sitemap_release.py --status
    python3 scripts/brickplan_en_sitemap_release.py --dry-run
    python3 scripts/brickplan_en_sitemap_release.py                 # +100 today
    python3 scripts/brickplan_en_sitemap_release.py --count 50 --force

    # production (ledger kept outside the git tree so `git checkout -f` cannot
    # rewind it — see EXPOSURE_LEDGER_PATH in internal/router/router.go):
    EXPOSURE_LEDGER_PATH=/home/ubuntu/project/brickplans/shared/sitemap-exposure.txt \
        python3 scripts/brickplan_en_sitemap_release.py

Exit codes: 0 ok (including "nothing to do"), 1 usage/IO error.
"""
from __future__ import annotations

import argparse
import datetime as dt
import os
import re
import shutil
import sys
import tempfile

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DEFAULT_CONTENT_DIR = os.path.join(REPO_ROOT, "backend-go", "content", "en", "blog")
DEFAULT_LEDGER = os.environ.get(
    "EXPOSURE_LEDGER_PATH",
    os.path.join(REPO_ROOT, "backend-go", "content", "en", "sitemap-exposure.txt"),
)
TITLE_LINE = "# brickplan.cn English sitemap exposure ledger"
HEADER_RE = re.compile(r"^#\s*last release\s+(\d{4}-\d{2}-\d{2}T[\d:]+Z)")
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")


def read_ledger(path):
    """Return (title, header_lines, slugs) — comments preserved, slugs in file order."""
    if not os.path.exists(path):
        return TITLE_LINE, [], []
    title, headers, slugs, seen = TITLE_LINE, [], [], set()
    with open(path, encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip().lstrip("\ufeff")
            if not line:
                continue
            if line.startswith("#"):
                if "last release" in line:
                    headers.append(line)
                elif TITLE_LINE not in line:
                    headers.append(line)
                continue
            if line not in seen:
                seen.add(line)
                slugs.append(line)
    return title, headers, slugs


def last_release_date(headers):
    """UTC date of the newest `# last release ...` line, or None."""
    newest = None
    for line in headers:
        m = HEADER_RE.match(line)
        if not m:
            continue
        try:
            stamp = dt.datetime.fromisoformat(m.group(1).replace("Z", "+00:00"))
        except ValueError:
            continue
        day = stamp.astimezone(dt.timezone.utc).date()
        if newest is None or day > newest:
            newest = day
    return newest


def pool_slugs(content_dir):
    """Every English article slug on disk, alphabetical (the rollout order)."""
    if not os.path.isdir(content_dir):
        sys.exit(f"content dir not found: {content_dir}")
    slugs = [
        name[:-3]
        for name in os.listdir(content_dir)
        if name.endswith(".md") and SLUG_RE.match(name[:-3])
    ]
    return sorted(slugs)


def atomic_write(path, text):
    """Write text to path atomically (temp file + fsync + os.replace)."""
    directory = os.path.dirname(path) or "."
    os.makedirs(directory, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=directory, prefix=".sitemap-exposure.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(text)
            fh.flush()
            os.fsync(fh.fileno())
        os.chmod(tmp, 0o644)
        os.replace(tmp, path)  # atomic on the same filesystem
        # Persist the rename itself, otherwise a power loss can resurrect the
        # old inode and silently rewind a day of rollout.
        dfd = os.open(directory, os.O_RDONLY)
        try:
            os.fsync(dfd)
        finally:
            os.close(dfd)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise


def render(title, headers, slugs):
    lines = [title]
    lines.extend(headers)
    lines.extend(slugs)
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--count", type=int, default=100, help="slugs to release (default 100)")
    ap.add_argument("--ledger", default=DEFAULT_LEDGER, help=f"ledger path (default {DEFAULT_LEDGER})")
    ap.add_argument("--content-dir", default=DEFAULT_CONTENT_DIR, help="English content dir")
    ap.add_argument("--seed-from", default=None,
                    help="copy this file to --ledger if the ledger does not exist yet "
                         "(default: the in-repo seed, skipped when --ledger IS the seed)")
    ap.add_argument("--no-seed", action="store_true", help="never seed a missing ledger")
    ap.add_argument("--dry-run", action="store_true", help="print the batch, write nothing")
    ap.add_argument("--force", action="store_true", help="release again even if today already released")
    ap.add_argument("--status", action="store_true", help="print progress and exit")
    args = ap.parse_args()

    ledger = os.path.abspath(args.ledger)
    content_dir = os.path.abspath(args.content_dir)
    seed = args.seed_from or os.path.join(
        REPO_ROOT, "backend-go", "content", "en", "sitemap-exposure.txt"
    )

    # First run on a new host: without a ledger the gate is OFF (everything
    # advertised), and creating it with only 100 slugs would deflate exposure
    # from 1,536 to 100. Seed from the repo file instead, then append.
    seeded = False
    if not os.path.exists(ledger) and not args.no_seed and os.path.abspath(seed) != ledger:
        if os.path.exists(seed):
            if args.dry_run:
                seeded = True  # implied by the simulation below
            else:
                os.makedirs(os.path.dirname(ledger) or ".", exist_ok=True)
                shutil.copyfile(seed, ledger)
                seeded = True

    title, headers, released = read_ledger(ledger if not seeded or not args.dry_run else seed)
    pool = pool_slugs(content_dir)
    known = set(pool)
    released_set = set(released)
    pending = [s for s in pool if s not in released_set]
    stale = sorted(released_set - known)  # slugs with no markdown any more

    today = dt.datetime.now(dt.timezone.utc).date()
    already_today = last_release_date(headers) == today

    print(f"ledger      : {ledger}{'  (seeded from repo)' if seeded else ''}")
    print(f"articles    : {len(pool)} on disk")
    print(f"released    : {len(released_set)}")
    print(f"pending     : {len(pending)}")
    print(f"last release: {last_release_date(headers) or 'never'}")
    if stale:
        print(f"⚠ stale     : {len(stale)} released slugs have no .md ({', '.join(stale[:3])}…)")

    if args.status:
        if pending:
            print(f"next batch  : {', '.join(pending[:min(args.count, 10)])}"
                  f"{' …' if len(pending) > 10 else ''}")
        return 0

    if not pending:
        print("nothing to release — every English article is already advertised")
        return 0
    if already_today and not args.force:
        print(f"already released today ({today}); nothing to do (use --force to override)")
        return 0

    if args.count <= 0:
        sys.exit("--count must be positive")
    batch = pending[:args.count]
    header = (
        f"# last release {dt.datetime.now(dt.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')}: "
        f"+{len(batch)} slugs (total {len(released_set) + len(batch)})"
    )
    new_headers = [header] + headers
    new_slugs = sorted(released_set | set(batch))
    text = render(title, new_headers, new_slugs)

    if args.dry_run:
        print(f"dry-run     : would add {len(batch)} → total {len(new_slugs)}")
        for slug in batch[:10]:
            print(f"  + {slug}")
        if len(batch) > 10:
            print(f"  … and {len(batch) - 10} more")
        return 0

    atomic_write(ledger, text)
    print(f"released    : +{len(batch)} → total {len(new_slugs)} (gate picks it up on the next request)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
