# PRD-17: 运营数据埋点

| 字段 | 内容 |
|------|------|
| 功能 | 前端埋点 + 后端统计 API + 简易看板 |
| 优先级 | P2（上线后优化用） |
| 预估 | 0.5天 |

---

## 1. 产品目标

没有数据就没有运营。需要知道：多少人来了、看了什么、从哪来、有没有注册、有没有上传、有没有分享。用最轻量的方式（不接第三方 SDK）收集核心指标。

## 2. 核心指标（北极星指标 → 拆解）

```
北极星：周活图纸上传数

漏斗：
  访问 → 浏览 → 注册 → 上传 → 分享
  UV     PV     注册数  上传数  分享数
```

### 关键看板指标

| 指标 | 定义 | 实现方式 |
|------|------|---------|
| PV/UV | 页面访问量/独立访客 | 后端请求日志统计 |
| 新注册 | 每日注册数 | 数据库查询 |
| 上传数 | 每日新图纸数 | 数据库查询 |
| 收藏数 | 每日收藏操作数 | 数据库查询 |
| 评论数 | 每日评论数 | 数据库查询 |
| 分享数 | 每日分享点击 | 前端埋点 |
| 页面停留 | 各页面平均停留时长 | 前端埋点（可选） |

## 3. 实现方案

### 后端——统计 API

**`GET /api/stats/overview`** — 运营概览
```json
{
  "total_blueprints": 156,
  "total_users": 89,
  "total_comments": 420,
  "total_favorites": 2300,
  "today": {
    "new_users": 3,
    "new_blueprints": 5,
    "new_comments": 12,
    "page_views": 340
  }
}
```

**`GET /api/stats/daily?days=30`** — 日趋势
```json
{
  "days": [
    {"date": "2026-06-01", "pv": 120, "uv": 45, "registrations": 2, "uploads": 3}
  ]
}
```

### 前端埋点（轻量）

```js
// 页面进入时发送
const track = (event, data = {}) => {
  navigator.sendBeacon('/api/analytics', JSON.stringify({
    event,
    page: state.page,
    referrer: document.referrer,
    ...data,
    t: Date.now()
  }));
};

// 关键事件
track('page_view');                    // 每个页面加载
track('upload_start');                 // 进入上传页
track('upload_complete');              // 上传成功
track('favorite', { blueprint_id });  // 点击收藏
track('share', { blueprint_id, platform }); // 点击分享
track('search', { query });           // 搜索
```

### 后端 analytics 收集

**`POST /api/analytics`** — 接收埋点事件，写入简单的 events 表或日志文件。

MVP 阶段直接写入日志文件：
```python
# analytics_logger.py
import json, time
from pathlib import Path

def log_event(event: dict):
    line = json.dumps({**event, "timestamp": time.time()}) + "\n"
    Path("analytics.log").open("a").write(line)
```

后续可导入数据库分析，或接入专业工具。

## 4. 验收标准

- [ ] `/api/stats/overview` 返回总量+今日数据
- [ ] `/api/stats/daily` 返回日趋势
- [ ] 前端关键事件（page_view/upload/favorite/share）正常发送
- [ ] 埋点不影响正常使用（sendBeacon 异步不阻塞）

## 5. 涉及文件

| 操作 | 文件 |
|------|------|
| 新建 | `backend/app/api/analytics.py` |
| 新建 | `backend/app/api/stats.py` |
| 修改 | `backend/app/main.py`（注册路由） |
| 修改 | `frontend/src/main.js`（track函数+关键事件） |
