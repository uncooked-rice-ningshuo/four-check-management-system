package importjob

import (
	"context"
	"strconv"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/inspection"
)

// ValidateManualAttendance shares the exact reference, date and duplicate rules
// used by XLSX imports; it never creates an import batch or file.
func (a *AttendanceAdapter) ValidateManualAttendance(ctx context.Context, input inspection.ManualAttendanceInput, week academic.Week, actorID string) (inspection.AttendanceRecord, error) {
	result, err := a.Validate(ctx, AttendanceImportScope{TypeCode: inspection.InspectionTypeAttendance, ExcludeRecordID: input.ExcludeRecordID, SemesterID: input.SemesterID, WeekID: input.WeekID, InspectionTypeID: input.InspectionTypeID, DepartmentID: input.DepartmentID, ClassID: input.ClassID, Grade: input.Grade, WeekStart: week.StartDate, WeekEnd: week.EndDate, CreatedBy: actorID}, []ParsedRow{{RowNo: 1, Values: map[string]string{
		"student_no": input.StudentNo, "student_name": input.StudentName, "class_name": input.ClassName, "course_name": input.CourseName, "classroom_code": input.Classroom, "absence_date": input.Date, "absence_time": input.Time, "period": input.Period, "absence_hours": strconv.FormatFloat(input.AbsenceHours, 'f', -1, 64), "remark": input.Remark,
	}}})
	if err != nil {
		return inspection.AttendanceRecord{}, err
	}
	for _, v := range result.Rows[0].Errors {
		if v.Code == ErrorRecordAlreadyExists {
			return inspection.AttendanceRecord{}, inspection.ErrAttendanceDuplicate
		}
	}
	if result.InvalidCount > 0 {
		return inspection.AttendanceRecord{}, &inspection.ManualValidationError{Message: result.Rows[0].Errors[0].Message + "；" + result.Rows[0].Errors[0].Suggestion}
	}
	return result.Rows[0].Record, nil
}
