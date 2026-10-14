#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""CI 守卫：确认 GOOGLE_SA_JSON 存在且格式正确。

⚠️ repo 是 public —— 本脚本**只打印非敏感指纹**（project / client_email / private_key 长度），
绝不打印 JSON 原文或私钥。任何异常信息也经过脱敏。
"""
import base64
import json
import os
import sys


def main() -> int:
    raw = (os.environ.get("GOOGLE_SA_JSON") or "").strip()
    if not raw:
        print("::error title=缺少 Secret::repo Secret GOOGLE_SA_JSON 未配置或为空")
        return 1
    try:
        if not raw.lstrip().startswith("{"):
            raw = base64.b64decode(raw).decode("utf-8")
        d = json.loads(raw)
    except Exception:  # noqa: BLE001
        # 不回显原文，避免把私钥打进公开日志
        print("::error title=Secret 解析失败::GOOGLE_SA_JSON 既不是 JSON 也不是 base64(JSON)")
        return 1

    if d.get("type") != "service_account":
        print("::error title=Secret 格式错::GOOGLE_SA_JSON 不是 service_account 类型")
        return 1
    for k in ("client_email", "private_key", "token_uri"):
        if not d.get(k):
            print("::error title=Secret 缺字段::缺少 %s（不打印内容）" % k)
            return 1

    pk = d["private_key"]
    print("SA 载入 OK | project=%s | client_email=%s | private_key_len=%d | pem=%s | 内容不打印"
          % (d.get("project_id"), d.get("client_email"), len(pk),
             "OK" if pk.startswith("-----BEGIN PRIVATE KEY-----") else "格式可疑"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
