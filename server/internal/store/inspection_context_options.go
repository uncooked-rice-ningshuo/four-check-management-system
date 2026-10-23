package store

import (
	"context"
	"errors"
	"fmt"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// ListContextOptions projects only scopes with an authorized class in SQL.
// Historical records supply their original grade scope even after configuration changes.
func (p *Postgres) ListContextOptions(ctx context.Context, q inspection.ContextOptionQuery, a inspection.MutationAccess) (inspection.ContextOptionPage, error) {
	result := inspection.ContextOptionPage{EntryOptionPage: inspection.EntryOptionPage{Items: []inspection.EntryOption{}, Page: q.Page, PageSize: q.PageSize}}
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code=$1 AND ($2='view' OR enabled)`, q.TypeCode, q.Purpose).Scan(&result.InspectionTypeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, inspection.ErrInspectionRequest
	}
	if err != nil {
		return result, err
	}
	// Match the existing entry-options edit semantics: authorize the original
	// record before exposing any destination; management never permits creation.
	owner := a.ActorID
	if q.RecordID != "" {
		original, e := (&inspectionTx{tx: tx}).Read(ctx, q.RecordID, a, "mutate", false)
		if e != nil {
			return result, e
		}
		if original.Status != inspection.RecordStatusDraft || original.HasSubmission || original.HasReportLock {
			return result, inspection.ErrRecordNotEditable
		}
		if original.InspectionTypeID != result.InspectionTypeID {
			return result, inspection.ErrInspectionRequest
		}
		owner = original.CreatedBy
	}
	mode := "read"
	if q.Purpose == "entry" {
		mode = "mutate"
		rules := make([]permission.EffectivePermission, 0, len(a.Permissions))
		for _, r := range a.Permissions {
			if r.Action == permission.ActionInspectionEnter || (q.RecordID != "" && r.Action == permission.ActionInspectionManage) {
				rules = append(rules, r)
			}
		}
		a.Permissions = rules
	}
	args := []any{result.InspectionTypeID, owner}
	// Candidate creation scopes require enabled calendars, mappings and classes.
	candidates := `SELECT c.id AS id,c.id AS class_id,g.name AS grade_snapshot,w.semester_id,w.id AS week_id,ds.inspection_type_id,ds.department_id,$2::uuid AS created_by
 FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id JOIN academic_years y ON y.id=s.academic_year_id
 CROSS JOIN classes c JOIN grades g ON g.id=c.grade_id
 CROSS JOIN department_inspection_scopes ds JOIN departments d ON d.id=ds.department_id
 WHERE w.enabled AND s.enabled AND y.enabled AND c.enabled AND g.enabled AND d.enabled AND ds.enabled AND ds.can_enter AND ds.inspection_type_id=$1::uuid`
	candidates += ` UNION ALL SELECT c.id,c.id,g.name,w.semester_id,w.id,$1::uuid,NULL::uuid,$2::uuid
 FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id JOIN academic_years y ON y.id=s.academic_year_id
 CROSS JOIN classes c JOIN grades g ON g.id=c.grade_id
 WHERE w.enabled AND s.enabled AND y.enabled AND c.enabled AND g.enabled`
	if q.Purpose == "view" {
		candidates += ` UNION ALL SELECT id,class_id,grade_snapshot,semester_id,week_id,inspection_type_id,department_id,created_by FROM inspection_records WHERE inspection_type_id=$1::uuid AND deleted_at IS NULL AND class_id IS NOT NULL`
		if q.TypeCode == inspection.InspectionTypeDormitoryHygiene || q.TypeCode == inspection.InspectionTypeClassroomHygiene || q.TypeCode == inspection.InspectionTypePublicDormitoryHygiene {
			candidates += ` UNION ALL SELECT ir.id,rc.class_id,rc.grade_snapshot,ir.semester_id,ir.week_id,ir.inspection_type_id,ir.department_id,ir.created_by
 FROM inspection_records ir JOIN inspection_responsibility_classes rc ON rc.record_id=ir.id
 WHERE ir.inspection_type_id=$1::uuid AND ir.deleted_at IS NULL`
		}
	}
	guard := inspectionAccessSQL(a, mode, &args)
	base := ` FROM (` + candidates + `) ir JOIN semesters sem ON sem.id=ir.semester_id JOIN academic_years y ON y.id=sem.academic_year_id JOIN semester_weeks w ON w.id=ir.week_id LEFT JOIN departments d ON d.id=ir.department_id JOIN classes c ON c.id=ir.class_id WHERE ` + guard
	for _, filter := range []struct{ column, value string }{{"ir.semester_id", q.SemesterID}, {"ir.week_id", q.WeekID}, {"ir.department_id", q.DepartmentID}} {
		if filter.value != "" {
			args = append(args, filter.value)
			base += fmt.Sprintf(" AND %s=$%d::uuid", filter.column, len(args))
		}
	}
	if q.CollegeOnly {
		base += " AND ir.department_id IS NULL"
	}
	if q.Grade != "" {
		args = append(args, q.Grade)
		base += fmt.Sprintf(" AND ir.grade_snapshot=$%d", len(args))
	}
	columns := ""
	switch q.Kind {
	case "semesters":
		columns = "sem.id,y.name || ' / ' || sem.name AS label"
	case "weeks":
		columns = "w.id,'第' || w.week_no || '周（' || w.start_date::text || '—' || w.end_date::text || '）' AS label"
	case "departments":
		columns = fmt.Sprintf("COALESCE(d.id,'%s'::uuid) AS id,COALESCE(d.name,'校级') AS label", inspection.CollegeDepartmentOptionID)
	case "classes":
		columns = "c.id,c.name AS label"
	case "grades":
		columns = "ir.grade_snapshot AS id,ir.grade_snapshot AS label"
	default:
		return result, inspection.ErrInspectionRequest
	}
	gradeColumn := "''::text AS grade"
	penaltyColumn := "NULL::smallint AS attendance_penalty_multiplier"
	if q.Kind == "weeks" {
		penaltyColumn = "w.attendance_penalty_multiplier"
	}
	if q.Kind == "classes" {
		// Permission checks above retain historical grade snapshots; display metadata
		// uses the current class grade so each class still has one unique option ID.
		gradeColumn = "(SELECT name FROM grades WHERE id=c.grade_id) AS grade"
	}
	selection := "SELECT DISTINCT " + columns + "," + gradeColumn + "," + penaltyColumn + base
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM ("+selection+") options", args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := tx.Query(ctx, selection+fmt.Sprintf(" ORDER BY label,id LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var option inspection.EntryOption
		if err = rows.Scan(&option.ID, &option.Label, &option.Grade, &option.AttendancePenaltyMultiplier); err != nil {
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
