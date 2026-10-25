package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
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
	if result.TypeCode != inspection.InspectionTypeAttendance && result.TypeCode != inspection.InspectionTypeUnattended && result.TypeCode != inspection.InspectionTypeEveningSelfStudy && result.TypeCode != inspection.InspectionTypeMorning && result.TypeCode != inspection.InspectionTypeLateReturn && result.TypeCode != inspection.InspectionTypeDormitoryHygiene && result.TypeCode != inspection.InspectionTypeClassroomHygiene {
		return result, importjob.ErrPreviewStale
	}
	return result, nil
}

// lockCourseImportReferences locks enabled references and their relationships
// until commit. Existing preview snapshots stay unchanged; changed class/grade
// membership must be previewed again because it changes the authorization scope.
func lockCourseImportReferences(ctx context.Context, tx pgx.Tx, scope inspection.InspectionContext, record inspection.AttendanceRecord) error {
	occurred := record.OccurredAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	raw, err := json.Marshal(inspection.CourseAbsenceDetails{StudentID: record.StudentID, ClassID: record.ClassID, CourseID: record.CourseID, ClassroomID: record.ClassroomID, Date: occurred.Format("2006-01-02"), Time: occurred.Format("15:04:05"), Period: record.PeriodText, AbsenceHours: record.AbsenceHours, Remark: record.Remark})
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
