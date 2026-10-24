package store

import (
	"context"
	"errors"
	"fmt"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SaveManualAttendance keeps reference snapshots, the draft and audit atomic.
// Reference locks prevent edits between final validation and insertion; the
// existing active dedup index arbitrates concurrent manual/import requests.
func (p *Postgres) SaveManualAttendance(ctx context.Context, r inspection.AttendanceRecord, m audit.Mutation) (inspection.AttendanceRecord, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return inspection.AttendanceRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockAttendanceReferences(ctx, tx, r); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	multiplier, err := attendancePenaltyMultiplier(ctx, tx, r.WeekID)
	if err != nil {
		return inspection.AttendanceRecord{}, err
	}
	r.PenaltyMultiplier = &multiplier
	err = tx.QueryRow(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,violation_count,remark,source,status,dedup_key,row_version,created_by)
 VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,1,$13,'MANUAL','DRAFT',$14,1,$15) RETURNING created_at,updated_at`, r.ID, r.InspectionTypeID, r.SemesterID, r.WeekID, r.DepartmentID, r.StudentID, r.StudentNoSnapshot, r.StudentNameSnapshot, r.ClassID, r.ClassNameSnapshot, r.GradeSnapshot, r.OccurredAt, r.Remark, r.DedupKey, r.CreatedBy).Scan(&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "uq_inspection_records_active_dedup" {
			return inspection.AttendanceRecord{}, inspection.ErrAttendanceDuplicate
		}
		return inspection.AttendanceRecord{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id,penalty_multiplier) VALUES($1,$2,$3,$4,$5,$6)`, r.ID, r.CourseID, r.AbsenceHours, r.PeriodText, r.ClassroomID, multiplier); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT penalty_hours FROM attendance_details WHERE record_id=$1`, r.ID).Scan(&r.PenaltyHours); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	if err = insertAuditTx(ctx, tx, m.Metadata, m.Entry); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	return r, nil
}

func lockAttendanceReferences(ctx context.Context, tx pgx.Tx, r inspection.AttendanceRecord) error {
	departmentJoin := `JOIN departments d ON d.id=$8`
	departmentMapping := `JOIN department_inspection_scopes ds ON ds.department_id=d.id AND ds.inspection_type_id=it.id`
	departmentWhere := "AND d.enabled AND ds.enabled AND ds.can_enter"
	departmentLocks := ",d,ds"
	if r.DepartmentID == "" {
		departmentJoin, departmentMapping, departmentWhere, departmentLocks = "", "", "", ""
	}
	var valid bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT true FROM students s
 JOIN classes c ON c.id=s.class_id JOIN grades g ON g.id=c.grade_id
	JOIN courses co ON co.id=$4
 JOIN classrooms cr ON cr.id=$5 JOIN teaching_buildings tb ON tb.id=cr.teaching_building_id
 JOIN semester_weeks w ON w.id=$6 AND w.semester_id=$7
 JOIN semesters sem ON sem.id=w.semester_id JOIN academic_years y ON y.id=sem.academic_year_id
 %s
 JOIN inspection_types it ON it.id=$9 AND it.code='ATTENDANCE_RATE'
 %s
 WHERE s.id=$1 AND c.id=$2 AND g.name=$3 AND s.enabled AND c.enabled AND g.enabled
 AND co.enabled AND cr.enabled AND tb.enabled AND w.enabled AND sem.enabled AND y.enabled
 AND it.enabled %s
 AND s.student_no=$10 AND s.name=$11 AND c.name=$12
 AND co.name=$13 AND tb.name || ' ' || cr.room_no=$14
 AND ($15::timestamptz AT TIME ZONE 'Asia/Shanghai')::date BETWEEN w.start_date AND w.end_date
	 FOR SHARE OF s,c,g,co,cr,tb,w,sem,y,it%s`, departmentJoin, departmentMapping, departmentWhere, departmentLocks), r.StudentID, r.ClassID, r.GradeSnapshot, r.CourseID, r.ClassroomID, r.WeekID, r.SemesterID, r.DepartmentID, r.InspectionTypeID, r.StudentNoSnapshot, r.StudentNameSnapshot, r.ClassNameSnapshot, r.CourseName, r.ClassroomCode, r.OccurredAt).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return inspection.ErrInvalidAttendanceRecord
	}
	if err != nil {
		return err
	}
	_, _, validPeriod := inspection.ParseCoursePeriod(r.PeriodText)
	if !validPeriod {
		return inspection.ErrInvalidAttendanceRecord
	}
	return nil
}
