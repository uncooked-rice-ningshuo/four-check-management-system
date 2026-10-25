package importjob

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
)

// EveningSelfStudyAdapter validates no-course absences at the fixed ninth and
// tenth periods using the same authorized student, class and room references.
type EveningSelfStudyAdapter struct{ repository AttendanceReferenceRepository }

func NewEveningSelfStudyAdapter(repository AttendanceReferenceRepository) *EveningSelfStudyAdapter {
	return &EveningSelfStudyAdapter{repository: repository}
}

func (a *EveningSelfStudyAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypeEveningSelfStudy {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	refs, err := a.repository.LoadAttendanceReferences(ctx, attendanceLookup(scope, rows))
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load evening self-study references: %w", err)
	}
	if refs.WeekPenaltyMultiplier == nil {
		return InspectionValidationResult{}, inspection.ErrAttendancePenaltyUnconfigured
	}
	result := InspectionValidationResult{Rows: make([]InspectionValidationRow, 0, len(rows))}
	seen := map[string]int{}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		validated := validateEveningSelfStudyRow(scope, refs, row)
		if len(validated.Errors) == 0 {
			key := validated.Record.DedupKey
			if previous, exists := seen[key]; exists {
				validated.Errors = append(validated.Errors, validationError(ErrorDuplicateInBatch, "student_no", fmt.Sprintf("第 %d 行与第 %d 行为同一学生同一时段", row.RowNo, previous), "每名学生在同一日期和固定节次保留一条记录"))
			} else {
				seen[key] = row.RowNo
				keys = append(keys, key)
			}
		}
		result.Rows = append(result.Rows, validated)
	}
	existing, err := a.repository.FindExistingAttendanceKeys(ctx, scope, uniqueStrings(keys))
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("find existing evening self-study keys: %w", err)
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		if _, found := existing[row.Record.DedupKey]; found && row.Record.DedupKey != "" {
			row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "student_no", "该学生此日期的晚自习缺勤记录已存在", "核对已有记录，不重复导入同一日期和节次"))
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}

func validateEveningSelfStudyRow(scope AttendanceImportScope, refs AttendanceReferences, row ParsedRow) InspectionValidationRow {
	result := InspectionValidationRow{RowNo: row.RowNo, Errors: []ValidationError{}}
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	for _, field := range []string{"student_no", "student_name", "class_name", "classroom_code", "absence_date", "absence_time"} {
		if value(field) == "" {
			result.Errors = append(result.Errors, validationError(ErrorRequiredField, field, "必填字段不能为空", "按模板填写此字段"))
		}
	}
	student, class, referenceIssues := validateStudentImportReferences(scope, refs, row)
	result.Errors = append(result.Errors, referenceIssues...)
	classroom, classroomOK := refs.ClassroomsByCode[value("classroom_code")]
	if value("classroom_code") != "" && !classroomOK {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "classroom_code", "教室不存在或不在当前数据范围", "使用系统中的完整教室位置"))
	}
	if utf8.RuneCountInString(value("remark")) > 500 {
		result.Errors = append(result.Errors, validationError(ErrorTextTooLong, "remark", "备注不能超过500字", "缩短备注"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	details := inspection.EveningSelfStudyDetails{
		StudentID: student.ID, ClassID: class.ID, ClassroomID: classroom.ID,
		Date: value("absence_date"), Time: value("absence_time"), Period: "9-10", AbsenceHours: 2, Remark: value("remark"),
	}
	raw, err := json.Marshal(details)
	if err != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidWorkbook, "absence_date", "无法生成检查明细", "重新核对模板"))
		return result
	}
	detailContext := inspection.DetailContext{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, WeekStart: scope.WeekStart.Format("2006-01-02"), WeekEnd: scope.WeekEnd.Format("2006-01-02")}
	normalized, issues := inspection.ValidateDetails(scope.TypeCode, detailContext, raw)
	for _, issue := range issues {
		code, field, message := ErrorInvalidDate, "absence_date", "缺勤日期格式无效"
		if issue.Code == "OUTSIDE_WEEK" {
			code, message = ErrorOccurredOutsideWeek, "缺勤日期不在当前周次内"
		} else if strings.Contains(issue.Path, "time") {
			code, field, message = ErrorInvalidTime, "absence_time", "检查时间格式无效"
		} else if strings.Contains(issue.Path, "period") || strings.Contains(issue.Path, "absenceHours") {
			code, field, message = ErrorInvalidHours, "absence_date", "晚自习时段或缺勤学时不符合固定规则"
		}
		result.Errors = append(result.Errors, validationError(code, field, message, "晚自习固定为第 9–10 节、2 学时；日期须处于所选周次"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	inputDetails, err := json.Marshal(map[string]any{
		"studentId": details.StudentID, "classId": details.ClassID, "classroomId": details.ClassroomID,
		"date": details.Date, "time": details.Time, "remark": details.Remark,
	})
	if err != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidWorkbook, "absence_date", "无法生成检查明细", "重新核对模板"))
		return result
	}
	result.Record = InspectionImportRecord{
		SchemaVersion: 1, ID: uuid.NewString(), TypeCode: scope.TypeCode,
		Input:      inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, Details: inputDetails},
		References: inspection.InspectionReferences{Classes: []inspection.ClassReference{{ID: class.ID, Name: class.Name, Grade: class.Grade}}, Students: []inspection.StudentReference{{ID: student.ID, ClassID: student.ClassID, Number: student.StudentNo, Name: student.Name}}},
		CreatedBy:  scope.CreatedBy, ImportBatchID: scope.ImportBatchID, DedupKey: normalized.DuplicateKey,
	}
	return result
}
