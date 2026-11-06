-- 2026-11-05 无机课堂口径修复：计次落库 + 去重键统一（见 docs/plans/2026-11-05-template-field-alignment.md）。
-- 只回填无机课堂自身的记录明细与去重键，不改写其他检查类型、已提交批次与已冻结的通报快照。

-- 1. 去重键统一为「学期+周次+检查类型+学生+课程+业务日期」，与手工录入和导入预览使用同一算法：
--    sha256(concat_ws(chr(31), semester_id, week_id, inspection_type_id, student_id, course_id, 'YYYY-MM-DD'))。
--    历史行是按「班级+学生+日期」写入的旧键，导入查重按新键匹配不到，会让同一事件重复入库，因此必须回填。
--    同一学生同一天同一课程若已存在多条有效记录（旧键本就允许），只回填最早的一条，其余保持原值，
--    避免迁移在唯一索引上失败；这些记录本身是待人工合并的重复数据。
WITH recomputed AS (
    SELECT r.id,
           r.created_at,
           encode(sha256(convert_to(concat_ws(chr(31),
               r.semester_id::text,
               r.week_id::text,
               r.inspection_type_id::text,
               r.student_id::text,
               d.course_id::text,
               to_char(r.occurred_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')
           ), 'UTF8')), 'hex') AS new_key
    FROM inspection_records r
    JOIN attendance_details d ON d.record_id = r.id
    JOIN inspection_types t ON t.id = r.inspection_type_id AND t.code = 'UNATTENDED_CLASSROOM'
    WHERE r.deleted_at IS NULL
      AND r.occurred_at IS NOT NULL
      AND r.student_id IS NOT NULL
      AND d.course_id IS NOT NULL
), ranked AS (
    SELECT id, new_key, row_number() OVER (PARTITION BY new_key ORDER BY created_at, id) AS position
    FROM recomputed
)
UPDATE inspection_records r
SET dedup_key = ranked.new_key
FROM ranked
WHERE r.id = ranked.id
  AND ranked.position = 1
  AND r.dedup_key <> ranked.new_key;

-- 2. 历史无机课堂记录缺少违纪倍数与计次（手工与导入都没写），按记录所属周次的配置补一份快照。
--    这是当前配置而非当时的配置，仅用于让旧数据可被统计口径识别；回填后不再随周次配置变化。
UPDATE attendance_details d
SET penalty_multiplier = w.attendance_penalty_multiplier,
    penalty_count = w.attendance_penalty_multiplier
FROM inspection_records r
JOIN inspection_types t ON t.id = r.inspection_type_id AND t.code = 'UNATTENDED_CLASSROOM'
JOIN semester_weeks w ON w.id = r.week_id
WHERE d.record_id = r.id
  AND r.deleted_at IS NULL
  AND d.penalty_multiplier IS NULL
  AND w.attendance_penalty_multiplier IS NOT NULL;
