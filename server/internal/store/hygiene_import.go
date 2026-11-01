package store

import (
	"context"
	"encoding/json"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
)

// LoadHygieneImportObjects loads only named enabled objects from an authorized
// batch. Names are not identifiers: all matches are retained for ambiguity checks.
func (p *Postgres) LoadHygieneImportObjects(ctx context.Context, lookup importjob.HygieneImportLookup) (importjob.HygieneImportObjects, error) {
	result := importjob.HygieneImportObjects{Dormitories: map[string][]importjob.HygieneNamedReference{}, Classrooms: map[string][]importjob.HygieneNamedReference{}}
	type objectQuery struct {
		sql    string
		args   []any
		target map[string][]importjob.HygieneNamedReference
	}
	queries := []objectQuery{}
	switch lookup.TypeCode {
	case inspection.InspectionTypeDormitoryHygiene:
		queries = append(queries, objectQuery{`SELECT d.id,''::text,a.name || ' ' || b.name || ' ' || d.room_no FROM dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE d.enabled AND b.enabled AND a.enabled AND a.name || ' ' || b.name || ' ' || d.room_no=ANY($1::text[]) ORDER BY d.id`, []any{lookup.DormitoryNames}, result.Dormitories})
	case inspection.InspectionTypePublicDormitoryHygiene:
		queries = append(queries, objectQuery{`SELECT d.id,''::text,a.name || ' ' || b.name || ' ' || d.room_no FROM dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE d.enabled AND b.enabled AND a.enabled AND a.name || ' ' || b.name || ' ' || d.room_no=ANY($1::text[]) ORDER BY d.id`, []any{lookup.ResponsibleNames}, result.Dormitories})
	case inspection.InspectionTypeClassroomHygiene:
		queries = append(queries, objectQuery{`SELECT c.id,''::text,b.name || ' ' || c.room_no FROM classrooms c JOIN teaching_buildings b ON b.id=c.teaching_building_id WHERE c.enabled AND b.enabled AND b.name || ' ' || c.room_no=ANY($1::text[]) ORDER BY c.id`, []any{lookup.ClassroomNames}, result.Classrooms})
	default:
		return result, importjob.ErrInvalidBatch
	}
	for _, query := range queries {
		rows, err := p.Pool.Query(ctx, query.sql, query.args...)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var v importjob.HygieneNamedReference
			if err := rows.Scan(&v.ID, &v.ParentID, &v.Name); err != nil {
				rows.Close()
				return result, err
			}
			query.target[v.Name] = append(query.target[v.Name], v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// FindExistingHygieneKeys checks all responsibility rows instead of the nullable
// personal-record class column; one out-of-scope class hides the whole record.
func (p *Postgres) FindExistingHygieneKeys(ctx context.Context, scope importjob.AttendanceImportScope, keys []string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	if len(keys) == 0 {
		return result, nil
	}
	rows, err := p.Pool.Query(ctx, `SELECT r.dedup_key FROM inspection_records r WHERE r.semester_id=$1::uuid AND r.week_id=$2::uuid AND r.inspection_type_id=$3::uuid AND r.department_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid AND r.dedup_key=ANY($5::text[]) AND r.deleted_at IS NULL
 AND EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=r.id AND (($6<>'' AND rc.class_id<>NULLIF($6,'')::uuid) OR ($7<>'' AND rc.grade_snapshot<>$7)))`, scope.SemesterID, scope.WeekID, scope.InspectionTypeID, scope.DepartmentID, keys, scope.ClassID, scope.Grade)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = struct{}{}
	}
	return result, rows.Err()
}

// FindExistingDailyInspectionDuplicates compares normalized identities in one
// scoped query so import preview remains bounded for large workbooks.
func (p *Postgres) FindExistingDailyInspectionDuplicates(ctx context.Context, scope importjob.AttendanceImportScope, candidates []importjob.InspectionDuplicateCandidate) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	if len(candidates) == 0 {
		return result, nil
	}
	payload, err := json.Marshal(candidates)
	if err != nil {
		return nil, err
	}
	rows, err := p.Pool.Query(ctx, `
WITH candidates AS (
    SELECT * FROM jsonb_to_recordset($5::jsonb) AS c(
        key text, class_id uuid, student_id uuid, object_id uuid, date date,
        period_start smallint, period_end smallint
    )
)
SELECT c.key
FROM candidates c
JOIN inspection_records r ON r.semester_id=$1::uuid AND r.week_id=$2::uuid
    AND r.inspection_type_id=$3::uuid AND r.department_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid
    AND r.deleted_at IS NULL AND r.dedup_date=c.date
WHERE CASE $6
    WHEN 'ATTENDANCE_RATE' THEN r.dedup_class_id=c.class_id AND r.dedup_student_id=c.student_id
        AND r.dedup_period_start=c.period_start AND r.dedup_period_end=c.period_end
    WHEN 'MORNING_ATTENDANCE' THEN r.dedup_class_id=c.class_id AND r.dedup_student_id=c.student_id
    WHEN 'LATE_RETURN' THEN r.dedup_class_id=c.class_id AND r.dedup_student_id=c.student_id
    WHEN 'EVENING_SELF_STUDY' THEN r.dedup_class_id=c.class_id AND r.dedup_student_id=c.student_id
    WHEN 'DORMITORY_HYGIENE' THEN r.dedup_object_id=c.object_id
    WHEN 'CLASSROOM_HYGIENE' THEN r.dedup_object_id=c.object_id
    WHEN 'PUBLIC_DORMITORY_HYGIENE' THEN r.dedup_object_id=c.object_id
    ELSE false
END
AND CASE WHEN $6 IN ('DORMITORY_HYGIENE','CLASSROOM_HYGIENE','PUBLIC_DORMITORY_HYGIENE') THEN
    EXISTS (SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=r.id
        AND ($7='' OR rc.class_id=NULLIF($7,'')::uuid)
        AND ($8='' OR rc.grade_snapshot=$8))
ELSE ($7='' OR r.class_id=NULLIF($7,'')::uuid)
    AND ($8='' OR EXISTS (SELECT 1 FROM classes cl JOIN grades g ON g.id=cl.grade_id WHERE cl.id=r.class_id AND g.name=$8))
END
	ORDER BY c.key`, scope.SemesterID, scope.WeekID, scope.InspectionTypeID, scope.DepartmentID, payload, scope.TypeCode, scope.ClassID, scope.Grade)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = struct{}{}
	}
	return result, rows.Err()
}
