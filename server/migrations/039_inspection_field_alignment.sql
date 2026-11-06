-- 2026-11-05 检查字段对齐（见 docs/plans/2026-11-05-template-field-alignment.md）。
-- 只放宽/收紧写入契约，不改写历史记录与已冻结的通报快照。

-- 1. 无机课堂不再采集节次；到课率继续使用该列。
ALTER TABLE attendance_details ALTER COLUMN period_text DROP NOT NULL;

-- 2. 无机课堂改为「一条记录计一次」，与晚自习一致：基础 1 次乘周次倍数。
--    到课率仍使用 absence_hours 与生成的 penalty_hours，penalty_count 保持为空。
ALTER TABLE attendance_details
    ADD COLUMN penalty_count smallint
        CHECK (penalty_count IS NULL OR penalty_count = penalty_multiplier);

-- 3. 公共宿舍卫生不再采集「公共区域」，area_id 维持 037 之后的可空状态，不改约束。
