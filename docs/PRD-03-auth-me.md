# PRD-03: 当前用户信息接口

| 字段 | 内容 |
|------|------|
| 功能 | GET /api/auth/me 返回当前登录用户信息 |
| 优先级 | P0 |
| 预估 | 0.25天 |

---

## 1. 产品目标

前端需要获取当前登录用户的基本信息（用户名、头像、邮箱），用于：
- 导航栏显示用户名/头像
- 评论列表显示当前用户头像
- 个人主页入口

当前状态：`GET /api/auth/me` 返回 501 Not Implemented。前端通过 localStorage 读取 token 中的 user 信息，但缺少用户名等（注册/登录返回的 TokenResponse 不含 user 对象）。

## 2. 实现方案

### 后端 (`backend/app/api/auth.py`)

替换当前 501 实现：

```python
@router.get("/me", response_model=UserOut)
async def get_me(
    current_user: User = Depends(get_current_user),
):
    return _to_user_out(current_user)
```

- 已有 `get_current_user` 依赖（`deps.py`），从 JWT 提取用户
- 已有 `UserOut` schema
- 已有 `_to_user_out()` helper（在 `blueprints.py` 中），需提取为公共工具或复制

### 注册/登录返回值增强（可选）

注册和登录返回的 `TokenResponse` 目前只有 token。考虑同时返回 `user` 信息，前端可在 localStorage 存储。

## 3. 验收标准

- [ ] 带有效 token 请求 `/api/auth/me` 返回 200 + 用户信息
- [ ] 无 token 返回 401
- [ ] 过期 token 返回 401
- [ ] 返回字段：id, username, email, avatar_url, bio, created_at

## 4. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `backend/app/api/auth.py` |
