package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// lockInspectionImportContext repeats configuration validation in the confirmation
// transaction. Preview-time checks cannot protect a later write.
func lockInspectionImportContext(ctx context.Context, tx pgx.Tx, scope importjob.ConfirmScope) (inspection.InspectionContext, error) {
	result, err := (&inspectionTx{tx: tx}).Context(ctx, inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID})
	if errors.Is(err, inspection.ErrInvalidAttendanceRecord) {
		return result, importjob.ErrPreviewStale
	}
	if err != nil {
		return result, err
	}
	if result.TypeCode != inspection.InspectionTypeAttendance && result.TypeCode != inspection.InspectionTypeUnattended && result.TypeCode != inspection.InspectionTypeEveningSelfStudy && result.TypeCode != inspection.InspectionTypeMorning && result.TypeCode != inspection.InspectionTypeLateReturn && result.TypeCode != inspection.InspectionTypeDormitoryHygiene && result.TypeCode != inspection.InspectionTypeClassroomHygiene && result.TypeCode != inspection.InspectionTypePublicDormitoryHygiene {
		return result, importjob.ErrPreviewStale
	}
	return result, nil
}

// lockCourseImportReferences locks enabled references and their relationships
// until commit. Existing preview snapshots stay unchanged; changed class/grade
// membership must be previewed again because it changes the authorization scope.
func lockCourseImportReferences(ctx context.Context, tx pgx.Tx, scope inspection.InspectionContext, record inspection.AttendanceRecord) error {
	occurred := record.OccurredAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	// Rebuild the detail payload from the stored columns. 到课率 and 无机课堂 now answer to
	// different field sets (无机课堂 collects no period, hours or clock time), so drop every
	// key the catalog does not define: validation rejects unknown fields, which would abort
	// the whole confirmation and be reported as a stale preview.
	definition, err := inspection.LookupInspectionDefinition(scope.TypeCode)
	if err != nil {
		return err
	}
	details := map[string]any{
		"studentId": record.StudentID, "classId": record.ClassID, "courseId": record.CourseID,
		"classroomId": record.ClassroomID, "date": occurred.Format("2006-01-02"),
		"time": occurred.Format("15:04:05"), "period": record.PeriodText,
		"absenceHours": record.AbsenceHours, "remark": record.Remark,
	}
	known := make(map[string]bool, len(definition.Fields))
	for _, field := range definition.Fields {
		known[field.Key] = true
	}
	for key := range details {
		if !known[key] {
			delete(details, key)
		}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	if _, issues := inspection.ValidateDetails(scope.TypeCode, scope.DetailContext, raw); len(issues) > 0 {
		return importjob.ErrPreviewStale
	}
	var present bool
	err = tx.QueryRow(ctx, `SELECT true FROM students s
	JOIN classes c ON c.id=s.class_id JOIN grades g ON g.id=c.grade_id
	JOIN courses co ON co.id=$3
	JOIN classrooms cr ON cr.id=$4 JOIN teaching_buildings b ON b.id=cr.teaching_building_id
	WHERE s.id=$1 AND c.id=$2 AND g.name=$5 AND s.enabled AND c.enabled AND g.enabled AND co.enabled AND cr.enabled AND b.enabled
	FOR SHARE OF s,c,g,co,cr,b`, record.StudentID, record.ClassID, record.CourseID, record.ClassroomID, record.GradeSnapshot).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return importjob.ErrPreviewStale
	}
	return err
}

// authorizeImportedClass repeats current range checks inside the confirmation transaction.
func authorizeImportedClass(ctx context.Context, tx pgx.Tx, input importjob.ConfirmBatchInput, classID string) error {
	scope := permission.Scope{SemesterID: input.Scope.SemesterID, WeekID: input.Scope.WeekID, InspectionTypeID: input.Scope.InspectionTypeID, DepartmentID: input.Scope.DepartmentID, ClassID: classID, StageGrades: map[int]string{}}
	err := tx.QueryRow(ctx, `SELECT c.grade_id,COALESCE(c.major_id::text,''),s.academic_year_id FROM classes c CROSS JOIN semesters s WHERE c.id=$1 AND s.id=$2 FOR SHARE OF c,s`, classID, input.Scope.SemesterID).Scan(&scope.GradeID, &scope.MajorID, &scope.AcademicYearID)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT stage,grade_id FROM academic_year_grade_stages WHERE academic_year_id=$1 FOR SHARE`, scope.AcademicYearID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var stage int
		var grade string
		if err := rows.Scan(&stage, &grade); err != nil {
			rows.Close()
			return err
		}
		scope.StageGrades[stage] = grade
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !input.Permissions.Allowed(permission.ActionInspectionEnter, scope) {
		if permission.MissingStageMapping(input.Permissions.Permissions, scope) {
			return permission.ErrGradeMappingMissing
		}
		return importjob.ErrBatchScopeMismatch
	}
	return nil
}
