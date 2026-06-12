# PRD-09: 字体修复 & 错误页面

| 字段 | 内容 |
|------|------|
| 功能 | 中文字体优化 + 404/错误友好页面 |
| 优先级 | P1 |
| 预估 | 0.25天 |

---

## 1. 产品目标

积木分享网站面向中文用户，当前系统字体在中文下可能达不到预期效果。同时需要优雅的错误处理页面。

## 2. 字体修复

### 目标

确保中文在所有系统和设备上都能正常、美观地渲染。

### 方案

在 CSS 中使用系统字体栈，优先使用中文字体：

```css
body {
  font-family: 
    "PingFang SC",
    "Microsoft YaHei",
    "Hiragino Sans GB",
    "WenQuanYi Micro Hei",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    Roboto,
    sans-serif;
}
```

不需要引入外部字体文件 —— 用系统原生字体保证加载速度和兼容性。

### 验收

- [ ] 中文正常显示，无乱码/方块
- [ ] 段落、标题字体层次分明
- [ ] Windows / macOS / Linux / iOS / Android 下中文 OK

## 3. 错误页面

### 3.1 404 页面

新建 `frontend/404.html`：
```
🔍 404
你要找的图纸可能被拆掉了...
[返回首页]
```

### 3.2 后端错误处理

- 全局异常处理，统一 JSON 格式
- 生产环境不暴露敏感堆栈
- 定制 404 / 500 响应

### 3.3 前端错误处理

- API 请求失败时 toast 提示
- 加载失败 retry 按钮
- 网络断线提示

## 4. 验收标准

- [ ] 中文字体在各种环境下正常
- [ ] 404 页面友好展示
- [ ] API 异常返回统一 JSON 格式（不暴露堆栈）
- [ ] 前端请求失败有 toast 提示

## 5. 涉及文件

| 操作 | 文件 |
|------|------|
| 修改 | `frontend/src/main.css` |
| 新建 | `frontend/404.html` |
| 修改 | `backend/app/main.py`（异常处理） |
| 修改 | `backend/nginx/brickplans-nginx.conf`（404 指向） |
