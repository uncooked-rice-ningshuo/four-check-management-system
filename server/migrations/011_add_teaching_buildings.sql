-- Teaching buildings are maintained independently so classrooms reference a
-- stable server-managed ID instead of repeating free-text building names.
CREATE TABLE teaching_buildings (
    id uuid PRIMARY KEY,
    name varchar(64) NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO teaching_buildings(id, name)
SELECT gen_random_uuid(), building
FROM (SELECT DISTINCT trim(building) AS building FROM classrooms) existing
WHERE building <> '';

ALTER TABLE classrooms ADD COLUMN teaching_building_id uuid;
UPDATE classrooms classroom
SET teaching_building_id = building.id
FROM teaching_buildings building
WHERE building.name = trim(classroom.building);
ALTER TABLE classrooms ALTER COLUMN teaching_building_id SET NOT NULL;
ALTER TABLE classrooms ADD CONSTRAINT classrooms_teaching_building_fkey
    FOREIGN KEY (teaching_building_id) REFERENCES teaching_buildings(id);

ALTER TABLE classrooms DROP CONSTRAINT IF EXISTS classrooms_building_room_no_key;
ALTER TABLE classrooms DROP COLUMN building;
ALTER TABLE classrooms DROP COLUMN floor;
ALTER TABLE classrooms ADD CONSTRAINT classrooms_building_room_key
    UNIQUE(teaching_building_id, room_no);
CREATE INDEX idx_classrooms_teaching_building
    ON classrooms(teaching_building_id, enabled);
