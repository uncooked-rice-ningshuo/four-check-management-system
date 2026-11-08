package store

import (
	"context"
	"errors"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) GetReportGrades(ctx context.Context, semesterID string) (academic.ReportGradeScope, error) {
	var result academic.ReportGradeScope
	result.SemesterID = semesterID
	err := p.Pool.QueryRow(ctx, `SELECT COALESCE(scope.version,0)
FROM semesters sem LEFT JOIN semester_report_grade_scopes scope ON scope.semester_id=sem.id
WHERE sem.id=$1::uuid`, semesterID).Scan(&result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return academic.ReportGradeScope{}, academic.ErrReportGradesNotFound
	}
	if err != nil {
		return academic.ReportGradeScope{}, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT grade_id::text FROM semester_report_grades WHERE semester_id=$1::uuid ORDER BY grade_id`, semesterID)
	if err != nil {
		return academic.ReportGradeScope{}, err
	}
	defer rows.Close()
	result.GradeIDs = make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return academic.ReportGradeScope{}, err
		}
		result.GradeIDs = append(result.GradeIDs, id)
	}
	return result, rows.Err()
}

func (p *Postgres) ReplaceReportGrades(ctx context.Context, scope academic.ReportGradeScope, metadata audit.Metadata) (academic.ReportGradeScope, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return academic.ReportGradeScope{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize first-time scope creation and all subsequent replacements on the semester row.
	var semesterID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM semesters WHERE id=$1::uuid AND enabled FOR UPDATE`, scope.SemesterID).Scan(&semesterID); errors.Is(err, pgx.ErrNoRows) {
		return academic.ReportGradeScope{}, academic.ErrReportGradesNotFound
	} else if err != nil {
		return academic.ReportGradeScope{}, err
	}
	var before academic.ReportGradeScope
	before.SemesterID = semesterID
	err = tx.QueryRow(ctx, `SELECT version FROM semester_report_grade_scopes WHERE semester_id=$1::uuid`, semesterID).Scan(&before.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		before.Version = 0
	} else if err != nil {
		return academic.ReportGradeScope{}, err
	}
	if before.Version != scope.Version {
		return academic.ReportGradeScope{}, academic.ErrReportGradesConflict
	}
	rows, err := tx.Query(ctx, `SELECT grade_id::text FROM semester_report_grades WHERE semester_id=$1::uuid ORDER BY grade_id`, semesterID)
	if err != nil {
		return academic.ReportGradeScope{}, err
	}
	before.GradeIDs = make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return academic.ReportGradeScope{}, err
		}
		before.GradeIDs = append(before.GradeIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return academic.ReportGradeScope{}, err
	}
	rows.Close()
	var available int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM grades WHERE id=ANY($1::uuid[]) AND enabled`, scope.GradeIDs).Scan(&available); err != nil {
		return academic.ReportGradeScope{}, err
	}
	if available != len(scope.GradeIDs) {
		return academic.ReportGradeScope{}, academic.ErrReportGradeUnavailable
	}
	if _, err := tx.Exec(ctx, `INSERT INTO semester_report_grade_scopes(semester_id,version,updated_by) VALUES($1::uuid,$2,NULLIF($3,'')::uuid)
ON CONFLICT (semester_id) DO UPDATE SET version=$2,updated_by=NULLIF($3,'')::uuid,updated_at=now()`, semesterID, before.Version+1, metadata.ActorID); err != nil {
		return academic.ReportGradeScope{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM semester_report_grades WHERE semester_id=$1::uuid`, semesterID); err != nil {
		return academic.ReportGradeScope{}, err
	}
	for _, gradeID := range scope.GradeIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO semester_report_grades(semester_id,grade_id) VALUES($1::uuid,$2::uuid)`, semesterID, gradeID); err != nil {
			return academic.ReportGradeScope{}, err
		}
	}
	scope.Version = before.Version + 1
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionAcademicCalendarChanged, ResourceType: "semesterReportGrades", ResourceID: semesterID, Before: before, After: scope}); err != nil {
		return academic.ReportGradeScope{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return academic.ReportGradeScope{}, err
	}
	return scope, nil
}

func (p *Postgres) PreviewReportGrades(ctx context.Context, gradeIDs []string) (academic.ReportGradePreview, error) {
	rows, err := p.Pool.Query(ctx, `SELECT grade.id::text,grade.name,
  (SELECT count(*) FROM classes class WHERE class.grade_id=grade.id AND class.enabled),
  (SELECT count(*) FROM students student WHERE student.grade_id=grade.id AND student.enabled AND student.enrollment_status='ENROLLED'),
  (SELECT count(*) FROM students student LEFT JOIN classes class ON class.id=student.class_id AND class.grade_id=grade.id AND class.enabled
   WHERE student.grade_id=grade.id AND student.enabled AND student.enrollment_status='ENROLLED' AND class.id IS NULL)
FROM grades grade WHERE grade.id=ANY($1::uuid[]) AND grade.enabled ORDER BY grade.name,grade.id`, gradeIDs)
	if err != nil {
		return academic.ReportGradePreview{}, err
	}
	defer rows.Close()
	result := academic.ReportGradePreview{Grades: make([]academic.ReportGradePreviewRow, 0, len(gradeIDs))}
	for rows.Next() {
		var item academic.ReportGradePreviewRow
		if err := rows.Scan(&item.GradeID, &item.GradeName, &item.ClassCount, &item.StudentCount, &item.UnassignedCount); err != nil {
			return academic.ReportGradePreview{}, err
		}
		result.Grades = append(result.Grades, item)
		result.ClassCount += item.ClassCount
		result.StudentCount += item.StudentCount
		result.UnassignedCount += item.UnassignedCount
	}
	if err := rows.Err(); err != nil {
		return academic.ReportGradePreview{}, err
	}
	if len(result.Grades) != len(gradeIDs) {
		return academic.ReportGradePreview{}, academic.ErrReportGradeUnavailable
	}
	return result, nil
}
