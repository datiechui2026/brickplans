# WeChat Mini Program Deployment

The BrickPlan WeChat Mini Program (`miniprogram/`) is a browse-only client for
the community: home, explore, detail, user profile, and WeChat login. It talks
to the **same Go backend** as the web SPA - no separate API.

## 1. Backend: enable WeChat login

WeChat login is optional - the backend boots fine without it (the endpoint
returns 503). To enable it, set in `backend-go/.env`:

```
WECHAT_APPID=wx-your-mini-program-appid
WECHAT_APPSECRET=your-mini-program-app-secret
```

Get both from the [WeChat Mini Program admin console](https://mp.weixin.qq.com)
→ 开发 → 开发管理 → 开发设置. The **AppSecret is used only server-side** to call
`jscode2session` - it never reaches the mini program client.

Restart the backend. Verify:

```bash
# Should return 503 with WECHAT_APPID unset, 400 with it set (no code).
curl -X POST https://brickplan.cn/api/auth/wechat-login -H 'Content-Type: application/json' -d '{"code":"test"}'
```

The backend adds a `wechat_open_id` (unique, nullable) and `wechat_union_id`
column to `users` via GORM AutoMigrate on startup - no manual migration. A
WeChat-only account gets a synthesized `wx_<openid>@wechat.local` email and an
empty password hash, so it can only log in via WeChat (password login returns
"该账号请使用微信登录").

## 2. Mini Program: configure the API base

Edit `miniprogram/utils/config.js`:

```js
const API_BASE = 'https://brickplan.cn'   // your backend's public origin
```

## 3. WeChat console: whitelist domains

In mp.weixin.qq.com → 开发 → 开发管理 → 开发设置 → 服务器域名:

- **request合法域名**: add `https://brickplan.cn` (for `wx.request` → `/api/*`).
- **downloadFile合法域名**: add `https://brickplan.cn` (for `wx.downloadFile` → PDFs).
  If blueprint images/PDFs are served from a COS CDN, add that domain too.

`<image src>` can load any HTTPS URL without whitelisting, so `/uploads/`
images and COS avatars render without extra config.

> In development, tick "不校验合法域名、TLS 版本及 HTTPS 证书" in WeChat DevTools
> → 详情 → 本地设置 to skip the whitelist.

## 4. Open and preview

1. Open WeChat DevTools, import the `miniprogram/` directory.
2. Set the AppID (replace `REPLACE_WITH_YOUR_APPID` in `project.config.json`,
   or pick the AppID on import).
3. The AppID here **must match** the `WECHAT_APPID` configured on the backend.
4. Preview / 真机调试 to test on your phone.

## 5. Auth flow recap (differs from the web SPA)

- The mini program calls `wx.login()` (silent, no consent prompt) → gets a
  `code` → `POST /api/auth/wechat-login {code}`.
- The backend exchanges the code for `openid` via `jscode2session`, finds or
  creates the user, and returns `{access_token, refresh_token, user}`.
- **Both tokens live in `wx` storage** (mini programs can't rely on httpOnly
  cookies). Every request sends `Authorization: Bearer <access_token>`.
- On 401, the client calls `POST /api/auth/refresh {refresh_token}` (the
  backend accepts the refresh token in the body as a cookieless fallback,
  which it also uses to keep the web SPA's cookie path working unchanged) and
  retries once.

## 6. Submit for review

WeChat review requires:

- A **privacy policy** page - included at `pages/privacy/index`, linked from
  the login screen. Update the contact/owner info in
  `pages/privacy/index.wxml` before submitting.
- A legitimate **service category** matching a community/forum.
- All whitelisted domains must be HTTPS and ICP-filed.

Out of scope for this mini program (by design - browse-only): upload, edit,
account settings, notifications, admin, blog. Users who want those use the web
site; WeChat users get an auto-generated `微信用户_<hex>` name + preset avatar
on first login.
