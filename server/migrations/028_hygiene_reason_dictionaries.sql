-- Inspection reason lists are independent per inspection type. Keep existing
-- administrator edits when a database already contains any of these entries.
INSERT INTO dictionary_types (id, code, name, description, system, enabled) VALUES
    (gen_random_uuid(), 'DORMITORY_HYGIENE_REASON', '宿舍卫生不合格原因', '宿舍卫生检查专用', true, true),
    (gen_random_uuid(), 'CLASSROOM_HYGIENE_REASON', '教室卫生不合格原因', '教室卫生检查专用', true, true),
    (gen_random_uuid(), 'PUBLIC_DORMITORY_HYGIENE_REASON', '公寓公共区域卫生不合格原因', '公寓公共区域卫生检查专用', true, true)
ON CONFLICT (code) DO NOTHING;

WITH reasons(type_code, value, label, sort_order) AS (VALUES
    ('DORMITORY_HYGIENE_REASON', 'FLOOR', '地面不洁', 10),
    ('DORMITORY_HYGIENE_REASON', 'BED', '床铺不整', 20),
    ('DORMITORY_HYGIENE_REASON', 'DESK', '桌面杂乱', 30),
    ('DORMITORY_HYGIENE_REASON', 'TRASH', '垃圾未清理', 40),
    ('DORMITORY_HYGIENE_REASON', 'ODOR', '室内异味', 50),
    ('DORMITORY_HYGIENE_REASON', 'OTHER', '其他', 90),
    ('CLASSROOM_HYGIENE_REASON', 'FLOOR', '地面不洁', 10),
    ('CLASSROOM_HYGIENE_REASON', 'DESK', '桌椅不整', 20),
    ('CLASSROOM_HYGIENE_REASON', 'TRASH', '垃圾未清理', 30),
    ('CLASSROOM_HYGIENE_REASON', 'BOARD', '黑板未清理', 40),
    ('CLASSROOM_HYGIENE_REASON', 'OTHER', '其他', 90),
    ('PUBLIC_DORMITORY_HYGIENE_REASON', 'FLOOR', '地面不洁', 10),
    ('PUBLIC_DORMITORY_HYGIENE_REASON', 'TRASH', '垃圾未清理', 20),
    ('PUBLIC_DORMITORY_HYGIENE_REASON', 'WALL', '墙面污损', 30),
    ('PUBLIC_DORMITORY_HYGIENE_REASON', 'ODOR', '公共区域异味', 40),
    ('PUBLIC_DORMITORY_HYGIENE_REASON', 'OTHER', '其他', 90)
)
INSERT INTO dictionary_items (id, type_id, value, label, sort_order, enabled)
SELECT gen_random_uuid(), types.id, reasons.value, reasons.label, reasons.sort_order, true
FROM reasons JOIN dictionary_types AS types ON types.code = reasons.type_code
ON CONFLICT (type_id, value) DO NOTHING;

-- Existing rows remain readable; new writes must supply free text for OTHER.
ALTER TABLE dorm_hygiene_details ADD CONSTRAINT dorm_hygiene_other_description_required
    CHECK (reason_code IS DISTINCT FROM 'OTHER' OR length(btrim(description)) > 0) NOT VALID;
ALTER TABLE classroom_hygiene_details ADD CONSTRAINT classroom_hygiene_other_description_required
    CHECK (reason_code IS DISTINCT FROM 'OTHER' OR length(btrim(description)) > 0) NOT VALID;
ALTER TABLE public_dormitory_hygiene_details ADD CONSTRAINT public_dormitory_hygiene_other_description_required
    CHECK (reason_code IS DISTINCT FROM 'OTHER' OR length(btrim(description)) > 0) NOT VALID;
