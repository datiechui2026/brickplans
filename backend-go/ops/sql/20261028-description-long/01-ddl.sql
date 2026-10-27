-- 工单 SEO-20261026-02 批2 —— blueprints 增加两个长描述列
-- 目标库: brickplans (MySQL 8.0.45)
-- 表规模: 2135 行（其中已发布 2134）/ 数据 2.5MB + 索引 1.1MB
--
-- 为什么是两列而不是复用 description:
--   description 同时供卡片列表、API 摘要、meta description 使用，长度上限和语义都
--   不同；长文 250-500 字塞进去会把列表和搜索结果撑爆。两列独立，互不覆盖。
--
-- 为什么把列追加在表尾（不写 AFTER）:
--   MySQL 8.0.12 起 ADD COLUMN 支持 INSTANT；8.0.29 起才支持「INSTANT + 任意位置」。
--   追加在表尾是 8.0.12+ 全版本都成立的 INSTANT 路径，列顺序对 GORM 与查询无影响。
--
-- 锁与耗时（实测见 README）:
--   ALGORITHM=INSTANT 只改数据字典，不重建表、不拷贝行，持有 MDL 的时间是毫秒级，
--   DML 不会被阻塞。若哪天 MySQL 拒绝 INSTANT（版本过旧/表有全文索引等），语句会
--   直接报错退出而不是静默降级成锁表 COPY —— 这是故意的。
--
-- 执行前必须先备份: bash 02-backup.sh

ALTER TABLE blueprints
  ADD COLUMN description_long    TEXT NULL,
  ADD COLUMN description_long_en TEXT NULL,
  ALGORITHM=INSTANT;

-- 回读校验（应返回 2 行，类型 text，Null=YES）
SELECT column_name, column_type, is_nullable
  FROM information_schema.columns
 WHERE table_schema = DATABASE() AND table_name = 'blueprints'
   AND column_name IN ('description_long','description_long_en');
