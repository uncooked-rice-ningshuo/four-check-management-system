#!/bin/sh
set -eu

psql -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);
SQL

# Older development volumes were created before the migration ledger existed.
# Baseline only migrations whose resulting schema is already present, so the
# destructive local-data reset in 009 is never replayed on an upgraded volume.
ledger_count="$(psql -Atqc "SELECT count(*) FROM schema_migrations")"
existing_schema="$(psql -Atqc "SELECT to_regclass('public.accounts') IS NOT NULL")"
if [ "$ledger_count" = "0" ] && [ "$existing_schema" = "t" ]; then
    psql -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO schema_migrations(version)
SELECT '001_init.sql' WHERE to_regclass('public.accounts') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '002_admin_foundation.sql' WHERE to_regclass('public.departments') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '003_phase_one_completion.sql' WHERE to_regclass('public.master_data_changes') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '004_storage_audit.sql' WHERE to_regclass('public.audit_logs') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '005_phase_two_attendance_import.sql' WHERE to_regclass('public.import_batches') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '006_expand_faculty_code.sql'
WHERE EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='public' AND table_name='faculties' AND column_name='code'
      AND character_maximum_length >= 64
) OR (
    to_regclass('public.faculties') IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema='public' AND table_name='faculties' AND column_name='code'
    )
)
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '007_add_grades_master_data.sql' WHERE to_regclass('public.grades') IS NOT NULL
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '008_add_dormitory_hierarchy.sql'
WHERE to_regclass('public.dormitory_buildings') IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema='public' AND table_name='dormitories' AND column_name='dormitory_building_id'
  )
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '009_server_managed_master_data.sql'
WHERE to_regclass('public.course_schedules') IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema='public' AND table_name='courses' AND column_name='code'
  )
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '010_master_data_integrity.sql'
WHERE EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname='course_schedules_no_overlap'
)
ON CONFLICT DO NOTHING;
INSERT INTO schema_migrations(version)
SELECT '011_add_teaching_buildings.sql'
WHERE to_regclass('public.teaching_buildings') IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema='public' AND table_name='classrooms' AND column_name='teaching_building_id'
  )
  AND NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema='public' AND table_name='classrooms' AND column_name='floor'
  )
ON CONFLICT DO NOTHING;
SQL
fi

migration_dir="${MIGRATIONS_DIR:-/migrations}"

for file in "$migration_dir"/*.sql; do
    version="$(basename "$file")"
    case "$version" in
        *[!0-9A-Za-z._-]*) echo "invalid migration filename: $version" >&2; exit 1 ;;
    esac

    applied="$(psql -Atqc "SELECT 1 FROM schema_migrations WHERE version = '$version'")"
    if [ "$applied" = "1" ]; then
        echo "skip $version (already applied)"
        continue
    fi

    echo "apply $version"
    {
        printf 'BEGIN;\n'
        cat "$file"
        printf "\nINSERT INTO schema_migrations(version) VALUES ('%s');\nCOMMIT;\n" "$version"
    } | psql -v ON_ERROR_STOP=1
done
