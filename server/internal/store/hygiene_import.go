package store

import (
	"context"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
)

// LoadHygieneImportObjects loads only named enabled objects from an authorized
// batch. Names are not identifiers: all matches are retained for ambiguity checks.
func (p *Postgres) LoadHygieneImportObjects(ctx context.Context, lookup importjob.HygieneImportLookup) (importjob.HygieneImportObjects, error) {
	result := importjob.HygieneImportObjects{Dormitories: map[string][]importjob.HygieneNamedReference{}, Classrooms: map[string][]importjob.HygieneNamedReference{}, Areas: map[string][]importjob.HygieneNamedReference{}, Reasons: map[string][]importjob.HygieneNamedReference{}}
	type objectQuery struct {
		sql    string
		args   []any
		target map[string][]importjob.HygieneNamedReference
	}
	queries := []objectQuery{}
	switch lookup.TypeCode {
	case inspection.InspectionTypeDormitoryHygiene:
		queries = append(queries, objectQuery{`SELECT d.id,''::text,a.name || ' ' || b.name || ' ' || d.room_no FROM dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE d.enabled AND b.enabled AND a.enabled AND a.name || ' ' || b.name || ' ' || d.room_no=ANY($1::text[]) ORDER BY d.id`, []any{lookup.DormitoryNames}, result.Dormitories})
	case inspection.InspectionTypeClassroomHygiene:
		queries = append(queries, objectQuery{`SELECT c.id,''::text,b.name || ' ' || c.room_no FROM classrooms c JOIN teaching_buildings b ON b.id=c.teaching_building_id WHERE c.enabled AND b.enabled AND b.name || ' ' || c.room_no=ANY($1::text[]) ORDER BY c.id`, []any{lookup.ClassroomNames}, result.Classrooms})
		queries = append(queries, objectQuery{`SELECT ar.id,ar.classroom_id,ar.name FROM classroom_hygiene_areas ar JOIN classrooms c ON c.id=ar.classroom_id JOIN teaching_buildings b ON b.id=c.teaching_building_id WHERE ar.enabled AND c.enabled AND b.enabled AND ar.name=ANY($1::text[]) AND b.name || ' ' || c.room_no=ANY($2::text[]) ORDER BY ar.id`, []any{lookup.AreaNames, lookup.ClassroomNames}, result.Areas})
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
	rows, err := p.Pool.Query(ctx, `SELECT i.id,i.value,i.label FROM dictionary_items i JOIN dictionary_types t ON t.id=i.type_id WHERE t.code=$1 AND t.enabled AND i.enabled AND i.label=ANY($2::text[]) ORDER BY i.id`, lookup.TypeCode+"_REASON", lookup.ReasonNames)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var v importjob.HygieneNamedReference
		if err := rows.Scan(&v.ID, &v.Code, &v.Name); err != nil {
			return result, err
		}
		result.Reasons[v.Name] = append(result.Reasons[v.Name], v)
	}
	return result, rows.Err()
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
