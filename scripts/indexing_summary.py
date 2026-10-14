#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 --json-out 的运行摘要渲染成 GitHub Step Summary 的 markdown。

判读口径（回执/验收用）：
  - 逐条 2xx  → 提交成功
  - 逐条 403 PERMISSION_DENIED → GSC 授权未完成（需站点所有者把 SA 加为「所有者」）
  - 非空值     → 本次未拿到 token 就失败（看上方日志）
"""
import json
import os
import sys

SA_EMAIL_FALLBACK = "brickplan-indexing@brickplan-indexing.iam.gserviceaccount.com"
SUMMARY = os.environ.get("INDEXING_SUMMARY", "/tmp/indexing-summary.json")


def main() -> int:
    print("## brickplan.cn · Google Indexing API 运行结果\n")
    print("| 项 | 值 |")
    print("|---|---|")
    print("| 触发方式 | `%s` |" % (os.environ.get("GITHUB_EVENT_NAME", "unknown")))
    print("| sitemap | https://brickplan.cn/sitemap.xml（递归展开 index） |")

    if not os.path.exists(SUMMARY):
        print("| 结果 | ⚠️ 未生成 summary —— 脚本在拿到 token 前即失败（见运行日志） |")
        print("\n### 判读\n- 检查 Secret `GOOGLE_SA_JSON` 是否为完整 service_account JSON；"
              "或 runner 无法访问 oauth2.googleapis.com。")
        return 0

    try:
        d = json.load(open(SUMMARY, encoding="utf-8"))
    except Exception as exc:  # noqa: BLE001
        print("| 结果 | ⚠️ summary 解析失败：%s |" % exc)
        return 0

    codes = d.get("http_codes") or {}
    print("| SA | `%s` |" % (d.get("sa") or SA_EMAIL_FALLBACK))
    print("| 计划提交 | %s 条 |" % d.get("planned"))
    print("| 成功（逐条 2xx） | **%s 条** |" % d.get("ok"))
    print("| 失败 | %s 条 |" % d.get("fail"))
    print("| 逐条 HTTP 状态分布 | `%s` |" % json.dumps(codes, ensure_ascii=False))
    print("| GSC 未授权（403） | %s |" % ("**是 ❌**" if d.get("permission_denied") else "否"))
    print("| 耗时 | %ss |" % d.get("seconds"))
    print("| state 累计已提交 | %s 条 |" % d.get("state_total"))

    print("\n### 结论判读\n")
    if d.get("permission_denied"):
        print("- ❌ **逐条 403 PERMISSION_DENIED → GSC 授权未完成**（不是代码问题）")
        print("- 需站点所有者在 Search Console 把 `%s` 加为**所有者**："
              % (d.get("sa") or SA_EMAIL_FALLBACK))
        print("  Search Console → 设置 → 用户和权限 → 添加用户 → 粘贴上面的 SA 邮箱 → 权限选「所有者」")
    elif d.get("ok") and not d.get("fail"):
        print("- ✅ **全部 %s 条 Google 真实返回 2xx**（逐条状态取自 multipart body，非整体状态）" % d.get("ok"))
    elif codes.get("429") or codes.get("403"):
        print("- ⚠️ 命中配额/权限限制，剩余 URL 已留给下次（state 已保存）")
    else:
        print("- ⚠️ 部分失败：成功 %s / 失败 %s" % (d.get("ok"), d.get("fail")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
