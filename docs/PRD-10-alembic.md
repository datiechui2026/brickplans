# PRD-10: Alembic 数据库迁移

| 字段 | 内容 |
|------|------|
| 功能 | 数据库迁移工具配置与初始迁移 |
| 优先级 | P1 |
| 预估 | 0.5天 |

---

## 1. 产品目标

当前数据库由 SQLAlchemy `create_all` 创建，缺少版本化迁移能力。引入 Alembic 保障未来 schema 变更安全可追溯。

## 2. 实现要点

### 配置 Alembic

- `pip install alembic`
- `alembic init backend/migrations`
- 修改 `alembic.ini` →
  - `sqlalchemy.url` 指向实际数据库
  - 或从环境变量读取
- 修改 `env.py` → 导入 SQLAlchemy 模型，支持异步

### 生成初始迁移

```bash
alembic revision --autogenerate -m "Initial migration"
alembic upgrade head
```

### 启动脚本

修改 `docker-compose.yml` 或启动脚本：
```yaml
command: >
  sh -c "alembic upgrade head && uvicorn app.main:app ..."
```

## 3. 验收标准

- [ ] Alembic 环境配置完成
- [ ] 初始迁移生成且包含所有模型表
- [ ] `alembic upgrade head` 可正常运行
- [ ] 启动命令自动执行迁移
- [ ] 已有数据不受影响

## 4. 涉及文件

| 操作 | 文件 |
|------|------|
| 新建 | `backend/migrations/` |
| 修改 | `backend/alembic.ini` |
| 修改 | `backend/pyproject.toml`（加依赖） |
| 修改 | `docker-compose.yml`（启动命令） |
