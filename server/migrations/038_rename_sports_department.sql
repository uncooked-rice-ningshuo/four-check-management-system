-- 002 may already be recorded on existing databases; update its seeded label forward.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM departments WHERE code = 'SPORTS') THEN
        RAISE EXCEPTION 'department code SPORTS is missing; cannot apply department rename';
    END IF;

    IF EXISTS (SELECT 1 FROM departments WHERE code <> 'SPORTS' AND name = '文体部') THEN
        RAISE EXCEPTION 'department name 文体部 is already assigned to another department';
    END IF;

    UPDATE departments
    SET name = '文体部', updated_at = now()
    WHERE code = 'SPORTS' AND name <> '文体部';
END;
$$;
