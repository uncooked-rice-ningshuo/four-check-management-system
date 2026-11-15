package store

import (
	"context"
	"errors"

	"four-check-management-system/server/internal/academic"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) GetReportGrades(ctx context.Context, semesterID string) (academic.ReportGradeScope, error) {
	var result academic.ReportGradeScope
	result.SemesterID = semesterID
	err := p.Pool.QueryRow(ctx, `SELECT year.version
FROM semesters sem JOIN academic_years year ON year.id=sem.academic_year_id
WHERE sem.id=$1::uuid`, semesterID).Scan(&result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return academic.ReportGradeScope{}, academic.ErrReportGradesNotFound
	}
	if err != nil {
		return academic.ReportGradeScope{}, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT grade_id::text FROM academic_year_grade_stages WHERE academic_year_id=(SELECT academic_year_id FROM semesters WHERE id=$1::uuid) ORDER BY grade_id`, semesterID)
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
