# WeChat Mini Program Release Guide

How to take the BrickPlan mini program from code to a published version that
real WeChat users can open. This complements `docs/deployment-miniprogram.md`
(which covers one-time setup) - this guide is about the **version → upload →
review → publish** cycle you repeat for each release.

The mini program is **browse-only** (home / explore / detail / user / privacy +
WeChat login). Anything beyond that (upload, edit, settings, notifications,
admin, blog) lives on the web site only.

## 0. Roles & where things happen

| Step | Where | Who |
|---|---|---|
| Configure, build, upload code | WeChat DevTools (本地) | developer |
| Set version + release notes | mp.weixin.qq.com → 版本管理 | admin |
| Submit for review | mp.weixin.qq.com → 版本管理 | admin |
| Publish after approval | mp.weixin.qq.com → 版本管理 | admin |

You need **admin** rights on the mini program in mp.weixin.qq.com to submit or
publish. The AppID used in DevTools must match the one configured on the
backend (`WECHAT_APPID`).

## 1. Pre-release checklist

Run through this every release - most review rejections come from skipping it.

- [ ] **Backend (production)**: `WECHAT_APPID` / `WECHAT_APPSECRET` set in
      `backend-go/.env`, backend restarted, `/api/auth/wechat-login` reachable.
- [ ] **API base**: `miniprogram/utils/config.js` `API_BASE` points at the
      production origin (HTTPS, ICP-filed), NOT a dev IP.
- [ ] **Domains whitelisted** in mp.weixin.qq.com → 开发 → 开发管理 → 开发设置 →
      服务器域名:
      - `request合法域名`: production origin (for `/api/*`)
      - `downloadFile合法域名`: production origin (for PDF download)
      - If images/PDFs come from a COS CDN, add that host to both.
- [ ] **AppID**: `miniprogram/project.config.json` `appid` matches the WeChat
      AppID (not `REPLACE_WITH_YOUR_APPID`).
- [ ] **Privacy policy**: review `pages/privacy/index.wxml` - update the
      contact/owner line and "最后更新" date. WeChat review checks this.
- [ ] **No dev leftovers**: in DevTools, 详情 → 本地设置, **uncheck**
      "不校验合法域名…" for a final real-config test (production rejects
      non-whitelisted domains).
- [ ] **Smoke test on 真机**: login, browse home/explore, open a detail, like +
      favorite + comment, open a PDF, view a profile. All must work against the
      production backend.

## 2. Bump the version

WeChat mini program versions use `major.minor.patch` (e.g. `1.0.0`). There is no
version field in the code - you set it at upload time in DevTools (step 3) and
it's tracked in the WeChat backend.

Suggested convention for BrickPlan:

- `1.0.0` → first published browse-only release.
- Patch (`1.0.1`) → bug fixes only.
- Minor (`1.1.0`) → new pages/features (e.g. when notifications/upload are added).
- Major (`2.0.0` → breaking UX changes.

Keep a changelog: append a section to this file (see **Release log** below) for
each version with its release notes (Chinese, since users see them).

## 3. Upload code (上传)

1. Open WeChat DevTools, import `miniprogram/`, pick the production AppID.
2. Top-right toolbar → **上传** (Upload).
3. Fill in:
   - **版本号**: e.g. `1.0.0`.
   - **项目备注**: short internal note (not shown to users).
4. Confirm. The code becomes a **开发版** (development version) visible in
   mp.weixin.qq.com → 管理 → 版本管理 → 开发版本列表.

> The upload includes only the `miniprogram/` directory's code + assets - it
> does **not** include your `backend-go/` or `frontend/`. Backend changes are
> deployed separately (see `docs/deployment-go.md`).

## 4. Test on 体验版 (experience version) before review

Don't submit for review cold - test the exact build first.

1. In 版本管理 → 开发版本列表, find the upload from step 3.
2. Click **选为体验版**. This pins it as the 体验版.
3. In 管理 → 成员管理, add **体验成员** (up to 15 by default). Only members can
   open the 体验版.
4. Have them open it via the WeChat mini program "体验版" entry and run the
   smoke test from step 1 **against production**.

The 体验版 uses the same domains/config as the eventual release, so this is your
real acceptance test. Fix bugs → re-upload (step 3) → re-select as 体验版.

## 5. Submit for review (提交审核)

1. 版本管理 → 开发版本列表 → the tested build → **提交审核**.
2. Fill the review form:
   - **服务类目**: pick one matching a community/forum (e.g. 社区/论坛). A
     wrong category is the #1 rejection reason.
   - **功能页面**: point at the home page (or detail) as the representative page.
   - **标签**: optional keywords.
   - **版本描述**: user-facing release notes (Chinese).
3. Submit. Review is usually 1-2 business days, up to 7.

### Common rejection reasons & fixes

| Reason | Fix |
|---|---|
| 服务类目不匹配 | pick a community/forum category; align with the actual feature. |
| 域名未配置/非 HTTPS | whitelist domains (step 1); all must be HTTPS + ICP-filed. |
| 无隐私政策/未授权 | privacy page exists + linked from login; complete the platform's 隐私授权 flow. |
| 诱导登录/强制登录 | login is optional (browse works logged-out); don't block content behind login. |
| 测试账号 | if review needs login, provide a test WeChat account in the review form. WeChat-only accounts can't be "test accounts" - note in the form that login is via WeChat. |
| 内容存在风险 | ensure no UGC is shown unmoderated on the landing pages; reports go to the admin queue. |

If rejected, fix → re-upload → re-submit. The previous submission stays in the
review list for reference.

## 6. Publish (发布)

After review passes (状态: 审核通过):

1. 版本管理 → 审核版本列表 → **发布**.
2. Optional: schedule a release time (定时发布) or enable **分阶段发布**
   (gradual rollout, e.g. 10% → 50% → 100% over days) to limit blast radius.
3. Confirm. The version goes **线上** (live). Users get it on next open
   (WeChat checks for updates; you can also prompt a强制更新 - see below).

Within ~24h, search/discovery (小程序搜索) reflects the new version.

## 7. Force update (optional but recommended)

WeChat caches the mini program. To force users onto a new version after a
breaking change, add a check in `app.js` `onLaunch` using
`wx.getUpdateManager()`. This is optional for v1 (browse-only changes are
usually non-breaking) - add it when you ship a breaking schema/auth change.

## 8. Rollback

If a published version is broken:

1. 版本管理 → 线上版本 → **退回** (or pick a previous 审核通过 version → 发布).
2. WeChat serves the previous version while you fix and re-release.

Backend changes are **independent** of mini program versions - if a backend
deploy broke things, rollback the backend (see `docs/deployment-go.md`), not the
mini program.

## 9. Post-release monitoring

- mp.weixin.qq.com → 数据 → 访问数据: opens, UV/PV, retention.
- Backend logs: watch `/api/auth/wechat-login` 4xx/5xx rates and `jscode2session`
  failures (usually a misconfigured AppSecret or expired code).
- Watch for `errcode` from WeChat (logged server-side): `40029` = invalid code
  (retry), `40013` = invalid AppID, `40125` = invalid AppSecret.

## Release log

Append a section per release. Example:

### 1.0.0 — (date)
- First published version of the BrickPlan mini program.
- Browse-only: home, explore, detail, user profile, privacy.
- WeChat login, like / favorite / comment / report.
- PDF opened via system viewer.
