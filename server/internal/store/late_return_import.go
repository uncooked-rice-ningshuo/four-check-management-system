package store

import (
	"context"
	"four-check-management-system/server/internal/importjob"
)

// LoadLateReturnDormitories resolves assigned rooms only for requested, scoped students.
func (p *Postgres) LoadLateReturnDormitories(ctx context.Context, lookup importjob.AttendanceReferenceLookup) (map[string]importjob.LateReturnDormitoryReference, error) {
	result := map[string]importjob.LateReturnDormitoryReference{}
	rows, err := p.Pool.Query(ctx, `SELECT s.id,d.id,a.name || ' ' || b.name || ' ' || d.room_no
 FROM students s JOIN classes c ON c.id=s.class_id JOIN grades g ON g.id=c.grade_id
 JOIN dormitories d ON d.id=s.dormitory_id JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id
 WHERE s.enabled AND c.enabled AND g.enabled AND d.enabled AND b.enabled AND a.enabled
 AND s.student_no=ANY($1::text[]) AND ($2='' OR c.id=NULLIF($2,'')::uuid) AND ($3='' OR g.name=$3)`, lookup.StudentNos, lookup.ClassID, lookup.Grade)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var studentID string
		var dorm importjob.LateReturnDormitoryReference
		if err = rows.Scan(&studentID, &dorm.ID, &dorm.Name); err != nil {
			return result, err
		}
		result[studentID] = dorm
	}
	return result, rows.Err()
}
