-- 工单 02 批2 验收查询。全部只读。
-- 用法: mysql -uroot brickplans < 04-verify.sql

-- V1 列存在且类型正确（期望 2 行: text/YES/text/YES）
SELECT column_name, column_type, is_nullable, column_default
  FROM information_schema.columns
 WHERE table_schema = DATABASE() AND table_name = 'blueprints'
   AND column_name IN ('description_long','description_long_en')
 ORDER BY column_name;

-- V2 覆盖率（灌库后跑）
SELECT COUNT(*)                                        AS total,
       SUM(description_long    IS NOT NULL)            AS zh_filled,
       SUM(description_long_en IS NOT NULL)            AS en_filled,
       SUM(description_long IS NULL AND description_long_en IS NULL) AS both_null
  FROM blueprints;

-- V3 长度分布：目标 250-500 字，落在外面的要人看一眼（工具只警告不改写）
SELECT 'zh' AS lang, COUNT(*) AS out_of_range
  FROM blueprints
 WHERE description_long IS NOT NULL
   AND (CHAR_LENGTH(description_long) < 250 OR CHAR_LENGTH(description_long) > 500)
UNION ALL
SELECT 'en', COUNT(*)
  FROM blueprints
 WHERE description_long_en IS NOT NULL
   AND (CHAR_LENGTH(description_long_en) < 250 OR CHAR_LENGTH(description_long_en) > 500);

SELECT 'zh' AS lang, MIN(CHAR_LENGTH(description_long)) AS min_len,
       ROUND(AVG(CHAR_LENGTH(description_long)),1) AS avg_len,
       MAX(CHAR_LENGTH(description_long)) AS max_len
  FROM blueprints WHERE description_long IS NOT NULL
UNION ALL
SELECT 'en', MIN(CHAR_LENGTH(description_long_en)),
       ROUND(AVG(CHAR_LENGTH(description_long_en)),1),
       MAX(CHAR_LENGTH(description_long_en))
  FROM blueprints WHERE description_long_en IS NOT NULL;

-- V4 原字段未被改写：把下面两行在变更前后各跑一次，fingerprint 必须完全一致。
--    （CRC32 求和 + 计数，GROUP_CONCAT 的 1024 字节上限不会踩到）
SELECT COUNT(*) AS rows_total,
       SUM(CRC32(CONCAT_WS('|', id, IFNULL(description,''), IFNULL(description_en,'')))) AS fingerprint
  FROM blueprints;

-- V5 抽样：随机 5 条已灌长文的记录（配合页面抽查）
SELECT id, CHAR_LENGTH(description_long) AS zh_len,
       CHAR_LENGTH(description_long_en) AS en_len,
       LEFT(description_long, 40) AS zh_head
  FROM blueprints
 WHERE description_long IS NOT NULL
 ORDER BY RAND() LIMIT 5;
