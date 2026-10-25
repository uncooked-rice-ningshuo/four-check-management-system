package store

import (
	"context"
	"fmt"

	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// ListEntryOptions filters candidate classes in SQL before joining student or course data.
// The synthetic record scope reuses the authoritative seven-dimension ALLOW/DENY predicate.
func (p *Postgres) ListEntryOptions(ctx context.Context, q inspection.EntryOptionQuery, a inspection.MutationAccess) (inspection.EntryOptionPage, error) {
	result := inspection.EntryOptionPage{Items: []inspection.EntryOption{}, Page: q.Page, PageSize: q.PageSize}
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	t := &inspectionTx{tx: tx}
	input := inspection.InspectionInput{SemesterID: q.SemesterID, WeekID: q.WeekID, InspectionTypeID: q.InspectionTypeID, DepartmentID: q.DepartmentID}
	scope, err := t.Context(ctx, input)
	if err != nil {
		return result, err
	}
	if scope.TypeCode != inspection.InspectionTypeAttendance && scope.TypeCode != inspection.InspectionTypeUnattended && scope.TypeCode != inspection.InspectionTypeLateReturn && scope.TypeCode != inspection.InspectionTypeMorning && scope.TypeCode != inspection.InspectionTypeDormitoryHygiene && scope.TypeCode != inspection.InspectionTypeClassroomHygiene && scope.TypeCode != inspection.InspectionTypePublicDormitoryHygiene {
		return result, inspection.ErrInspectionRequest
	}
	if scope.TypeCode == inspection.InspectionTypeMorning && q.Kind != "classes" && q.Kind != "students" {
		return result, inspection.ErrInspectionRequest
	}
	if (scope.TypeCode == inspection.InspectionTypeAttendance || scope.TypeCode == inspection.InspectionTypeUnattended) && q.Kind != "classes" && q.Kind != "students" && q.Kind != "courses" && q.Kind != "classrooms" {
		return result, inspection.ErrInspectionRequest
	}
	if scope.TypeCode == inspection.InspectionTypeLateReturn && q.Kind != "classes" && q.Kind != "students" && q.Kind != "dormitories" {
		return result, inspection.ErrInspectionRequest
	}
	if scope.TypeCode == inspection.InspectionTypeDormitoryHygiene && q.Kind != "classes" && q.Kind != "students" && q.Kind != "dormitory-objects" && q.Kind != "reason-codes" {
		return result, inspection.ErrInspectionRequest
	}
	if scope.TypeCode == inspection.InspectionTypeClassroomHygiene && q.Kind != "classes" && q.Kind != "students" && q.Kind != "classroom-objects" && q.Kind != "classroom-areas" && q.Kind != "reason-codes" {
		return result, inspection.ErrInspectionRequest
	}
	if scope.TypeCode == inspection.InspectionTypePublicDormitoryHygiene && q.Kind != "classes" && q.Kind != "students" && q.Kind != "public-areas" && q.Kind != "responsible-dormitories" && q.Kind != "reason-codes" {
		return result, inspection.ErrInspectionRequest
	}
	owner := a.ActorID
	if q.RecordID != "" {
		original, e := t.Read(ctx, q.RecordID, a, "mutate", false)
		if e != nil {
			return result, e
		}
		if original.Status != inspection.RecordStatusDraft || original.HasSubmission || original.HasReportLock {
			return result, inspection.ErrRecordNotEditable
		}
		if original.InspectionTypeID != q.InspectionTypeID {
			return result, inspection.ErrInspectionRequest
		}
		owner = original.CreatedBy
	} else {
		// Management grants permit editing, never creating a new inspection.
		rules := make([]permission.EffectivePermission, 0, len(a.Permissions))
		for _, rule := range a.Permissions {
			if rule.Action == permission.ActionInspectionEnter {
				rules = append(rules, rule)
			}
		}
		a.Permissions = rules
	}
	args := []any{q.SemesterID, q.WeekID, q.InspectionTypeID, q.DepartmentID, owner}
	guard := inspectionAccessSQL(a, "mutate", &args)
	base := ` FROM classes c JOIN grades g ON g.id=c.grade_id
 JOIN semesters sem ON sem.id=$1::uuid
 CROSS JOIN LATERAL (SELECT c.id AS id,c.id AS class_id,g.name AS grade_snapshot,
 $1::uuid AS semester_id,$2::uuid AS week_id,$3::uuid AS inspection_type_id,NULLIF($4,'')::uuid AS department_id,$5::uuid AS created_by) ir `
	where := ` WHERE c.enabled AND g.enabled AND ` + guard
	if q.ClassID != "" {
		args = append(args, q.ClassID)
		where += fmt.Sprintf(" AND c.id=$%d::uuid", len(args))
	}
	var columns string
	switch q.Kind {
	case "classes":
		columns = "c.id,c.name || '（' || CASE WHEN right(g.name, 1) = '级' THEN g.name ELSE g.name || '级' END || '）' AS label"
	case "students":
		base += " JOIN students s ON s.class_id=c.id "
		where += " AND s.enabled"
		columns = "s.id,s.name || '（' || s.student_no || '，' || c.name || '）' AS label"
	case "courses":
		// A course is a label in the inspection record; its options do not depend
		// on a selected class, date, classroom, semester schedule, or period.
		base += " CROSS JOIN courses co "
		where += " AND co.enabled"
		if q.Keyword != "" {
			args = append(args, q.Keyword)
			where += fmt.Sprintf(" AND co.name ILIKE '%%' || $%d || '%%'", len(args))
		}
		columns = "co.id,co.name AS label"
	case "classrooms":
		base += " CROSS JOIN classrooms cr JOIN teaching_buildings b ON b.id=cr.teaching_building_id "
		where += " AND cr.enabled AND b.enabled"
		columns = "cr.id,b.name || ' ' || cr.room_no AS label"
	case "dormitories":
		// Late-return writes require the student's assigned dormitory; never offer
		// the whole housing catalog or rooms belonging to an unrelated student.
		base += " JOIN students s ON s.class_id=c.id JOIN dormitories d ON d.id=s.dormitory_id JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id "
		args = append(args, q.StudentID)
		where += fmt.Sprintf(" AND s.id=$%d::uuid AND s.enabled AND d.enabled AND b.enabled AND da.enabled", len(args))
		columns = "d.id,da.name || ' ' || b.name || ' ' || d.room_no AS label"
	case "dormitory-objects":
		// A hygiene record is scoped by every selected responsibility class.
		// The object itself need not currently house a particular student.
		base += " CROSS JOIN dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id "
		where += " AND d.enabled AND b.enabled AND da.enabled"
		columns = "d.id,da.name || ' ' || b.name || ' ' || d.room_no AS label"
	case "reason-codes":
		args = append(args, scope.TypeCode+"_REASON")
		base += fmt.Sprintf(" JOIN dictionary_types dt ON dt.code=$%d JOIN dictionary_items di ON di.type_id=dt.id ", len(args))
		where += " AND dt.enabled AND di.enabled"
		columns = "di.value AS id,di.label AS label,di.sort_order AS order_hint"
	case "classroom-objects":
		base += " CROSS JOIN classrooms cr JOIN teaching_buildings b ON b.id=cr.teaching_building_id "
		where += " AND cr.enabled AND b.enabled"
		columns = "cr.id,b.name || ' ' || cr.room_no AS label"
	case "classroom-areas":
		args = append(args, q.ClassroomID)
		base += fmt.Sprintf(" JOIN classroom_hygiene_areas ar ON ar.classroom_id=$%d::uuid JOIN classrooms cr ON cr.id=ar.classroom_id JOIN teaching_buildings b ON b.id=cr.teaching_building_id ", len(args))
		where += " AND ar.enabled AND cr.enabled AND b.enabled"
		columns = "ar.id,b.name || ' ' || cr.room_no || ' / ' || ar.name AS label"
	case "public-areas":
		base += " CROSS JOIN public_dormitory_hygiene_areas ar JOIN dormitory_buildings b ON b.id=ar.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id "
		where += " AND ar.enabled AND b.enabled AND da.enabled"
		columns = "ar.id,da.name || ' ' || b.name || ' / ' || ar.floor_unit || ' / ' || ar.name AS label"
	case "responsible-dormitories":
		args = append(args, q.AreaID)
		base += fmt.Sprintf(" JOIN public_dormitory_hygiene_areas ar ON ar.id=$%d::uuid JOIN dormitory_buildings b ON b.id=ar.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id JOIN dormitories d ON d.dormitory_building_id=b.id ", len(args))
		where += " AND ar.enabled AND b.enabled AND da.enabled AND d.enabled"
		columns = "d.id,da.name || ' ' || b.name || ' ' || d.room_no AS label"
	default:
		return result, inspection.ErrInspectionRequest
	}
	selection := "SELECT DISTINCT " + columns + base + where
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM ("+selection+") options", args...).Scan(&result.Total); err != nil {
		return result, err
	}
	listQuery := selection
	orderBy := "label,id"
	if q.Kind == "reason-codes" {
		listQuery = "SELECT id,label,order_hint FROM (" + selection + ") options"
		orderBy = "order_hint,id"
	}
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := tx.Query(ctx, listQuery+fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var option inspection.EntryOption
		if q.Kind == "reason-codes" {
			var orderHint int
			err = rows.Scan(&option.ID, &option.Label, &orderHint)
		} else {
			err = rows.Scan(&option.ID, &option.Label)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Items = append(result.Items, option)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
