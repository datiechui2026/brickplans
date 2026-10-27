-- 工单 02 批2 —— 回滚：删掉两个长描述列。
--
-- !! 顺序硬约束 !!
--   先回滚二进制（回到不含 description_long 字段的版本），再执行本文件。
--   反过来做会让线上二进制带着已删列查询，全站详情页 1054 Unknown column。
--
-- 本语句会丢弃已灌入的长文内容。要么先跑 02-backup.sh，要么先
--   CREATE TABLE blueprints_longdesc_bak AS
--     SELECT id, description_long, description_long_en FROM blueprints;
-- 把内容单独留一份（几百 KB）。
--
-- DROP COLUMN 在 8.0.29+ 同样是 INSTANT；本例 2134 行的表预计毫秒级，DML 不阻塞。

-- 可选的内容兜底（执行前取消注释）
-- CREATE TABLE IF NOT EXISTS blueprints_longdesc_bak AS
--   SELECT id, description_long, description_long_en FROM blueprints
--    WHERE description_long IS NOT NULL OR description_long_en IS NOT NULL;

ALTER TABLE blueprints
  DROP COLUMN description_long_en,
  DROP COLUMN description_long,
  ALGORITHM=INSTANT;

SELECT COUNT(*) AS leftover_columns
  FROM information_schema.columns
 WHERE table_schema = DATABASE() AND table_name = 'blueprints'
   AND column_name IN ('description_long','description_long_en');
-- 期望 0
