package store

import (
	"context"
	"errors"
	"fmt"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"sort"
	"strings"
)

// mutationScopeSQL enforces ownership and action-specific ALLOW/DENY in SQL.
func mutationScopeSQL(access inspection.MutationAccess, args *[]any) string {
	if access.Unrestricted {
		return "TRUE"
	}
	scopeSet := func(action, effect string) string {
		var alternatives []string
		for _, rule := range access.Permissions {
			if rule.Action != action || rule.Effect != effect {
				continue
			}
			var terms []string
			for _, d := range []struct{ column, value string }{
				{"sem.academic_year_id", rule.Scope.AcademicYearID}, {"ir.semester_id", rule.Scope.SemesterID}, {"ir.week_id", rule.Scope.WeekID}, {"ir.inspection_type_id", rule.Scope.InspectionTypeID}, {"ir.department_id", rule.Scope.DepartmentID}, {"ir.class_id", rule.Scope.ClassID}, {"ir.grade_snapshot", rule.Scope.Grade},
			} {
				if d.value != "" {
					*args = append(*args, d.value)
					cast := "::uuid"
					if d.column == "ir.grade_snapshot" {
						cast = "::text"
					}
					terms = append(terms, fmt.Sprintf("%s=$%d%s", d.column, len(*args), cast))
				}
			}
			if len(terms) == 0 {
				alternatives = append(alternatives, "TRUE")
			} else {
				alternatives = append(alternatives, "("+strings.Join(terms, " AND ")+")")
			}
		}
		if len(alternatives) == 0 {
			return "FALSE"
		}
		return "(" + strings.Join(alternatives, " OR ") + ")"
	}
	*args = append(*args, access.ActorID)
	owner := fmt.Sprintf("ir.created_by=$%d::uuid", len(*args))
	ownAllow, ownDeny := scopeSet(permission.ActionInspectionEnter, permission.EffectAllow), scopeSet(permission.ActionInspectionEnter, permission.EffectDeny)
	manageAllow, manageDeny := scopeSet(permission.ActionInspectionManage, permission.EffectAllow), scopeSet(permission.ActionInspectionManage, permission.EffectDeny)
	return "((" + owner + " AND " + ownAllow + " AND NOT " + ownDeny + ") OR (" + manageAllow + " AND NOT " + manageDeny + "))"
}

const mutationRecordSelect = `SELECT ir.id,ir.inspection_type_id,ir.semester_id,ir.week_id,COALESCE(ir.department_id::text,''),
 COALESCE(ir.student_id::text,''),ir.student_no_snapshot,ir.student_name_snapshot,COALESCE(ir.class_id::text,''),ir.class_name_snapshot,ir.grade_snapshot,
 ad.course_id,c.name,COALESCE(ad.classroom_id::text,''),COALESCE(tb.name || ' ' || cr.room_no,''),
 ir.occurred_at,ad.absence_hours,ad.penalty_multiplier,ad.penalty_hours,ad.period_text,ir.remark,ir.source,ir.status,COALESCE(ir.import_batch_id::text,''),ir.dedup_key,ir.row_version,ir.created_by,ir.created_at,ir.updated_at,
 ir.submission_id IS NOT NULL,ir.locked_by_report_id IS NOT NULL
 FROM inspection_records ir JOIN attendance_details ad ON ad.record_id=ir.id JOIN courses c ON c.id=ad.course_id
 JOIN semesters sem ON sem.id=ir.semester_id JOIN inspection_types it ON it.id=ir.inspection_type_id
 LEFT JOIN classrooms cr ON cr.id=ad.classroom_id LEFT JOIN teaching_buildings tb ON tb.id=cr.teaching_building_id
 WHERE ir.id=$1::uuid AND ir.deleted_at IS NULL AND it.code='ATTENDANCE_RATE' AND `

type mutationQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readMutationRecord(ctx context.Context, q mutationQueryer, id string, access inspection.MutationAccess, lock bool) (inspection.AttendanceRecord, error) {
	args := []any{id}
	sql := mutationRecordSelect + mutationScopeSQL(access, &args)
	if lock {
		sql += " FOR UPDATE OF ir"
	}
	var r inspection.AttendanceRecord
	err := q.QueryRow(ctx, sql, args...).Scan(&r.ID, &r.InspectionTypeID, &r.SemesterID, &r.WeekID, &r.DepartmentID, &r.StudentID, &r.StudentNoSnapshot, &r.StudentNameSnapshot, &r.ClassID, &r.ClassNameSnapshot, &r.GradeSnapshot, &r.CourseID, &r.CourseName, &r.ClassroomID, &r.ClassroomCode, &r.OccurredAt, &r.AbsenceHours, &r.PenaltyMultiplier, &r.PenaltyHours, &r.PeriodText, &r.Remark, &r.Source, &r.Status, &r.ImportBatchID, &r.DedupKey, &r.RowVersion, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.HasSubmission, &r.HasReportLock)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, inspection.ErrRecordNotFound
	}
	return r, err
}
func (p *Postgres) ReadAttendanceForMutation(ctx context.Context, id string, a inspection.MutationAccess) (inspection.AttendanceRecord, error) {
	return readMutationRecord(ctx, p.Pool, id, a, false)
}
func (p *Postgres) UpdateAttendance(ctx context.Context, r inspection.AttendanceRecord, version int, a inspection.MutationAccess, m audit.Mutation) (inspection.AttendanceRecord, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return inspection.AttendanceRecord{}, err
	}
	defer tx.Rollback(ctx)
	original, err := readMutationRecord(ctx, tx, r.ID, a, true)
	if err != nil {
		return inspection.AttendanceRecord{}, err
	}
	if err = inspection.ValidateAttendanceEditRecord(original, version); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	if original.Status == inspection.RecordStatusReturned && (r.SemesterID != original.SemesterID || r.WeekID != original.WeekID || r.DepartmentID != original.DepartmentID || r.InspectionTypeID != original.InspectionTypeID) {
		return inspection.AttendanceRecord{}, inspection.ErrRecordNotEditable
	}
	if err = lockAttendanceReferences(ctx, tx, r); err != nil {
		return inspection.AttendanceRecord{}, err
	}
	multiplier := 0
	if original.HasSubmission && original.WeekID == r.WeekID && original.OccurredAt.In(r.OccurredAt.Location()).Format("2006-01-02") == r.OccurredAt.Format("2006-01-02") {
		if original.PenaltyMultiplier == nil {
			return inspection.AttendanceRecord{}, inspection.ErrAttendanceLegacyRuleUnknown
		}
		multiplier = *original.PenaltyMultiplier
	} else {
		multiplier, err = attendancePenaltyMultiplier(ctx, tx, r.WeekID)
		if err != nil {
			return inspection.AttendanceRecord{}, err
		}
	}
	r.PenaltyMultiplier = &multiplier
	// Keep origin metadata immutable even if an internal caller supplies a replacement.
	r.CreatedBy, r.Source, r.ImportBatchID, r.CreatedAt = original.CreatedBy, original.Source, original.ImportBatchID, original.CreatedAt
	args := []any{r.ID, version, r.InspectionTypeID, r.SemesterID, r.WeekID, r.DepartmentID, r.StudentID, r.StudentNoSnapshot, r.StudentNameSnapshot, r.ClassID, r.ClassNameSnapshot, r.GradeSnapshot, r.OccurredAt, r.Remark, r.DedupKey}
	guard := mutationScopeSQL(a, &args)
	sql := `UPDATE inspection_records ir SET inspection_type_id=$3,semester_id=$4,week_id=$5,department_id=NULLIF($6,'')::uuid,student_id=$7,student_no_snapshot=$8,student_name_snapshot=$9,class_id=$10,class_name_snapshot=$11,grade_snapshot=$12,occurred_at=$13,remark=$14,dedup_key=$15,row_version=ir.row_version+1,updated_at=now()
 FROM semesters sem WHERE sem.id=ir.semester_id AND ir.id=$1 AND ir.row_version=$2 AND ir.deleted_at IS NULL
 AND ((ir.status='DRAFT' AND ir.submission_id IS NULL) OR
 (ir.status='RETURNED' AND ir.submission_id IS NOT NULL AND EXISTS
 (SELECT 1 FROM department_submissions ds WHERE ds.id=ir.submission_id AND ds.status='RETURNED'
 AND ds.semester_id=$4 AND ds.week_id=$5 AND ds.department_id IS NOT DISTINCT FROM NULLIF($6,'')::uuid AND ds.inspection_type_id=$3)))
 AND ir.locked_by_report_id IS NULL AND ` + guard + ` RETURNING ir.row_version,ir.updated_at`
	if err = tx.QueryRow(ctx, sql, args...).Scan(&r.RowVersion, &r.UpdatedAt); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "uq_inspection_records_active_dedup" {
			return inspection.AttendanceRecord{}, inspection.ErrAttendanceDuplicate
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return inspection.AttendanceRecord{}, inspection.ErrRecordVersionConflict
		}
		return inspection.AttendanceRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE attendance_details SET course_id=$2,classroom_id=NULLIF($3,'')::uuid,absence_hours=$4,period_text=$5,penalty_multiplier=$6 WHERE record_id=$1`, r.ID, r.CourseID, r.ClassroomID, r.AbsenceHours, r.PeriodText, multiplier); err != nil {
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

// DeleteAttendance locks in stable order and commits the entire batch with audits.
func (p *Postgres) DeleteAttendance(ctx context.Context, items []inspection.VersionedRecord, a inspection.MutationAccess, metadata audit.Metadata) error {
	ordered := append([]inspection.VersionedRecord(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	records := make([]inspection.AttendanceRecord, 0, len(items))
	for _, item := range ordered {
		r, err := readMutationRecord(ctx, tx, item.ID, a, true)
		if err != nil {
			return err
		}
		if err = inspection.ValidateMutationRecord(r, item.Version); err != nil {
			return err
		}
		records = append(records, r)
	}
	for _, r := range records {
		args := []any{r.ID, r.RowVersion}
		guard := mutationScopeSQL(a, &args)
		tag, err := tx.Exec(ctx, `UPDATE inspection_records ir SET deleted_at=now(),updated_at=now(),row_version=ir.row_version+1 FROM semesters sem WHERE sem.id=ir.semester_id AND ir.id=$1 AND ir.row_version=$2 AND ir.deleted_at IS NULL AND ir.status='DRAFT' AND ir.submission_id IS NULL AND ir.locked_by_report_id IS NULL AND `+guard, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return inspection.ErrRecordVersionConflict
		}
		after := inspection.MutationAuditSnapshot(r)
		after["deleted"] = true
		after["rowVersion"] = r.RowVersion + 1
		if err = insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionInspectionRecordDeleted, ResourceType: "inspectionRecord", ResourceID: r.ID, Before: inspection.MutationAuditSnapshot(r), After: after}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
