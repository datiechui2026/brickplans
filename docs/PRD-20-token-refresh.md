# PRD-20: Token 自动刷新机制

## 问题
Access Token 有效期仅 30 分钟。过期后，任何需要登录的操作（收藏、上传、编辑等）都返回 `"Invalid or expired token"`，用户被迫重新登录。

## 根因
- 后端 `/api/auth/refresh` 端点已实现（用 refresh_token 换新 access_token）
- refresh_token 有效期 7 天
- 但前端 `request()` 函数收到 401 时直接抛异常，没有尝试刷新

## 方案

### 前端修改（仅 `frontend/src/api.js`）

1. **添加 `getRefreshToken()` 函数**
   ```
   function getRefreshToken() {
     try { return JSON.parse(localStorage.getItem('bp_auth') || '{}').refresh_token || null; }
     catch { return null; }
   }
   ```

2. **添加 `tryRefreshToken()` 函数**
   - 用 refresh_token 调 `POST /api/auth/refresh`
   - 成功 → 更新 localStorage 的 bp_auth，返回新的 access_token
   - 失败 → 清除 bp_auth，触发重新登录

3. **修改 `request()` 函数**
   - 当 `res.status === 401` 且当前有 refresh_token 时：
     a. 调 `tryRefreshToken()` 
     b. 成功 → 用新 token 重试原请求一次
     c. 失败 → 抛出错误（前端捕获后跳转登录）

### 注意事项
- 避免无限刷新循环：刷新请求本身返回 401 时直接失败
- 并发请求保护：如果多个请求同时 401，只触发一次刷新
- 刷新成功后不必重新渲染整个页面，只需 localStorage 更新即可

## 验收标准
1. 登录超过 30 分钟后，点击收藏 → 自动刷新 token → 收藏成功（用户无感知）
2. 刷新同时多个请求 401 → 只触发一次 token 刷新 → 所有请求重试成功
3. refresh_token 也过期（7天后）→ 提示"登录已过期，请重新登录" → 跳转到登录
4. 未登录用户点收藏 → 弹出登录引导（保持现有行为不变）

## 涉及文件
- `frontend/src/api.js` — 修改 request()、新增 tryRefreshToken()、getRefreshToken()

## 任务量估算
0.5 天（前端纯逻辑改动）
