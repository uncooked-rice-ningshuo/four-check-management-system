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

// MorningAdapter resolves authorized student/class references in bulk and keeps
// the approved student/day granularity, without synthesizing a time or subtype.
type MorningAdapter struct{ repository AttendanceReferenceRepository }

func NewMorningAdapter(repository AttendanceReferenceRepository) *MorningAdapter {
	return &MorningAdapter{repository: repository}
}

func (a *MorningAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypeMorning {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	refs, err := a.repository.LoadAttendanceReferences(ctx, attendanceLookup(scope, rows))
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load morning references: %w", err)
	}
	result := InspectionValidationResult{Rows: make([]InspectionValidationRow, 0, len(rows))}
	seen := map[string]int{}
	keys := []string{}
	for _, row := range rows {
		validated := validateMorningRow(scope, refs, row)
		if len(validated.Errors) == 0 {
			key := validated.Record.DedupKey
			if previous, exists := seen[key]; exists {
				validated.Errors = append(validated.Errors, validationError(ErrorDuplicateInBatch, "inspection_date", fmt.Sprintf("第 %d 行与第 %d 行为同一学生同一天", row.RowNo, previous), "每名学生每天保留一条记录"))
			} else {
				seen[key] = row.RowNo
				keys = append(keys, key)
			}
		}
		result.Rows = append(result.Rows, validated)
	}
	existing, err := a.repository.FindExistingAttendanceKeys(ctx, scope, keys)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("find existing morning keys: %w", err)
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		if _, found := existing[row.Record.DedupKey]; found && row.Record.DedupKey != "" {
			row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "inspection_date", "该学生当天已有早锻炼/早签记录", "核对已有记录，不重复导入同一天"))
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}

func validateMorningRow(scope AttendanceImportScope, refs AttendanceReferences, row ParsedRow) InspectionValidationRow {
	result := InspectionValidationRow{RowNo: row.RowNo, Errors: []ValidationError{}}
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	for _, field := range []string{"student_no", "student_name", "class_name", "inspection_date"} {
		if value(field) == "" {
			result.Errors = append(result.Errors, validationError(ErrorRequiredField, field, "必填字段不能为空", "按模板填写此字段"))
		}
	}
	student, class, referenceIssues := validateStudentImportReferences(scope, refs, row)
	result.Errors = append(result.Errors, referenceIssues...)
	if utf8.RuneCountInString(value("remark")) > 500 {
		result.Errors = append(result.Errors, validationError(ErrorTextTooLong, "remark", "备注不能超过500字", "缩短备注"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	details := inspection.MorningAttendanceDetails{StudentID: student.ID, ClassID: class.ID, Date: value("inspection_date"), AbsenceCount: 1, Remark: value("remark")}
	raw, err := json.Marshal(details)
	if err != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidWorkbook, "inspection_date", "无法生成检查明细", "重新核对模板"))
		return result
	}
	detailContext := inspection.DetailContext{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, WeekStart: scope.WeekStart.Format("2006-01-02"), WeekEnd: scope.WeekEnd.Format("2006-01-02")}
	normalized, issues := inspection.ValidateDetails(scope.TypeCode, detailContext, raw)
	for _, issue := range issues {
		code, message := ErrorInvalidDate, "检查日期格式无效"
		if issue.Code == "OUTSIDE_WEEK" {
			code, message = ErrorOccurredOutsideWeek, "检查日期不在当前周次内"
		}
		result.Errors = append(result.Errors, validationError(code, "inspection_date", message, "使用当前周次内的YYYY-MM-DD日期"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	result.Record = InspectionImportRecord{SchemaVersion: 1, ID: uuid.NewString(), TypeCode: scope.TypeCode, Input: inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, Details: raw}, References: inspection.InspectionReferences{Classes: []inspection.ClassReference{{ID: class.ID, Name: class.Name, Grade: class.Grade}}, Students: []inspection.StudentReference{{ID: student.ID, ClassID: student.ClassID, Number: student.StudentNo, Name: student.Name}}}, CreatedBy: scope.CreatedBy, ImportBatchID: scope.ImportBatchID, DedupKey: normalized.DuplicateKey}
	return result
}

// validateStudentImportReferences applies the shared personal-record boundary.
func validateStudentImportReferences(scope AttendanceImportScope, refs AttendanceReferences, row ParsedRow) (AttendanceStudentReference, AttendanceClassReference, []ValidationError) {
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	issues := []ValidationError{}
	student, studentOK := refs.StudentsByNo[value("student_no")]
	class, classOK := refs.ClassesByName[value("class_name")]
	if value("student_no") != "" && !studentOK {
		issues = append(issues, validationError(ErrorReferenceNotFound, "student_no", "学号不存在或不在当前数据范围", "核对启用学生及授权范围"))
	}
	if value("class_name") != "" && !classOK {
		issues = append(issues, validationError(ErrorReferenceNotFound, "class_name", "班级不存在或不在当前数据范围", "使用系统中的有效班级名称"))
	}
	if studentOK && student.Name != value("student_name") {
		issues = append(issues, validationError(ErrorReferenceMismatch, "student_name", "姓名与学号不一致", "使用学号对应姓名"))
	}
	if studentOK && classOK && student.ClassID != class.ID {
		issues = append(issues, validationError(ErrorReferenceMismatch, "class_name", "学生与班级不一致", "核对学生当前所在班级"))
	}
	if classOK && ((scope.ClassID != "" && scope.ClassID != class.ID) || (scope.Grade != "" && scope.Grade != class.Grade)) {
		issues = append(issues, validationError(ErrorReferenceMismatch, "class_name", "班级或年级不在当前授权范围", "使用当前有权录入的班级"))
	}
	return student, class, issues
}
