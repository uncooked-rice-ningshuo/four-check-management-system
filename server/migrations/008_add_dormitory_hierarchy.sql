CREATE TABLE IF NOT EXISTS dormitory_areas (
    id uuid PRIMARY KEY,
    code varchar(64) NOT NULL UNIQUE,
    name varchar(96) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dormitory_buildings (
    id uuid PRIMARY KEY,
    dormitory_area_id uuid NOT NULL REFERENCES dormitory_areas(id),
    code varchar(32) NOT NULL,
    name varchar(96) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(dormitory_area_id, code),
    UNIQUE(dormitory_area_id, name)
);

INSERT INTO dormitory_areas(id, code, name)
SELECT gen_random_uuid(), campus_area, campus_area
FROM (SELECT DISTINCT campus_area FROM dormitories WHERE campus_area <> '') values_to_migrate
ON CONFLICT DO NOTHING;

INSERT INTO dormitory_buildings(id, dormitory_area_id, code, name)
SELECT gen_random_uuid(), area.id, values_to_migrate.building_no, values_to_migrate.building_no
FROM (SELECT DISTINCT campus_area, building_no FROM dormitories WHERE campus_area <> '' AND building_no <> '') values_to_migrate
JOIN dormitory_areas area ON area.code = values_to_migrate.campus_area
ON CONFLICT DO NOTHING;

ALTER TABLE dormitories ADD COLUMN IF NOT EXISTS dormitory_building_id uuid;

UPDATE dormitories room
SET dormitory_building_id = building.id
FROM dormitory_buildings building
JOIN dormitory_areas area ON area.id = building.dormitory_area_id
WHERE room.dormitory_building_id IS NULL
  AND room.campus_area = area.code
  AND room.building_no = building.code;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'dormitories_dormitory_building_id_fkey') THEN
        ALTER TABLE dormitories ADD CONSTRAINT dormitories_dormitory_building_id_fkey
            FOREIGN KEY (dormitory_building_id) REFERENCES dormitory_buildings(id);
    END IF;
END $$;

ALTER TABLE dormitories ALTER COLUMN dormitory_building_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dormitories_building ON dormitories(dormitory_building_id, enabled);
