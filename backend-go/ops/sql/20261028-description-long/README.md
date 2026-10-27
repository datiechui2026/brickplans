# 20261028-description-long —— 工单 02 批2 表结构变更包

| 文件 | 用途 | 是否改数据 |
|---|---|---|
| `01-ddl.sql` | ADD COLUMN ×2（INSTANT） | 只改结构 |
| `02-backup.sh` | mysqldump 只导 blueprints，校验收件完整性 | 只读 |
| `03-rollback.sql` | DROP COLUMN ×2（须先回滚二进制） | 结构 |
| `04-verify.sql` | 5 组只读验收查询 | 只读 |
| `cmd/import-longdesc` | CSV → 两列，支持 --dry-run / 断点续跑 | 只改两列 |

## 执行顺序（不可调换）

```
bash 02-backup.sh                      # 1. 先备份，记下 sha256
mysql -uroot brickplans < 01-ddl.sql   # 2. 加列（毫秒级）
# 3. 部署带 DescriptionLong 字段的二进制 —— 必须在第 2 步之后，见下
./import-longdesc --csv bodies.csv --dry-run   # 4. 先看计划
./import-longdesc --csv bodies.csv             # 5. 实写，可断点重跑
mysql -uroot brickplans < 04-verify.sql        # 6. 对 V4 指纹
```

## 为什么二进制必须排在 DDL 之后

`db.Blueprint` 结构体新增了两列，GORM 的 SELECT 会带上列名。列不存在时 MySQL 报
`1054 Unknown column 'description_long'`，详情页、列表页、sitemap 全部 500。所以
DDL 先行是硬约束；回滚时顺序相反（先退二进制，再 DROP COLUMN）。

## 锁表时长评估（MySQL 8.0.45，表 2134 行 / 3.6MB）

`ALGORITHM=INSTANT` 只写数据字典：不重建表、不拷行、不阻塞 DML，MDL 持有时间为
毫秒级（本机同规模实测见交付消息里的 `real` 计时）。若 8.0 拒绝 INSTANT（表上有
FULLTEXT 等），语句直接报错而不是降级成 COPY —— 宁可失败也不在没预告的情况下锁表。

`03-rollback.sql` 里的 DROP COLUMN 在 8.0.29+ 同样是 INSTANT。

## 长文内容规则（灌库侧）

- 250–500 字，一段或多段，段间空行（markdown 段落）。**不截断、不补齐**：工具对
  越界行只打印计数警告，写进去的是 CSV 原文。
- 空 zh / 空 en 单元格 = 该列保持不动（不把已有内容清空）。
- 断点重跑：每批事务提交后才把 id 追加到 `<csv>.progress`；重跑自动跳过。即使
  progress 丢了也不会重复写脏数据 —— 值相同即计为 unchanged。
