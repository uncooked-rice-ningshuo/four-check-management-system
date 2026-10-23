package importjob

import (
	"context"
	"encoding/json"
	"fmt"
	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
	"strings"
	"unicode/utf8"
)

// LateReturnDormitoryReference is the active dormitory assigned to a scoped student.
type LateReturnDormitoryReference struct{ ID, Name string }

// LateReturnReferenceRepository loads only the students requested in an authorized batch.
type LateReturnReferenceRepository interface {
	AttendanceReferenceRepository
	LoadLateReturnDormitories(context.Context, AttendanceReferenceLookup) (map[string]LateReturnDormitoryReference, error)
}

// LateReturnAdapter preserves individual timestamped events and dormitory membership.
type LateReturnAdapter struct{ repository LateReturnReferenceRepository }

func NewLateReturnAdapter(repository LateReturnReferenceRepository) *LateReturnAdapter {
	return &LateReturnAdapter{repository: repository}
}
func (a *LateReturnAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	if scope.TypeCode != inspection.InspectionTypeLateReturn {
		return InspectionValidationResult{}, ErrInvalidBatch
	}
	lookup := attendanceLookup(scope, rows)
	refs, err := a.repository.LoadAttendanceReferences(ctx, lookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load late-return references: %w", err)
	}
	dorms, err := a.repository.LoadLateReturnDormitories(ctx, lookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load late-return dormitories: %w", err)
	}
	result := InspectionValidationResult{Rows: make([]InspectionValidationRow, 0, len(rows))}
	seen := map[string]int{}
	keys := []string{}
	for _, row := range rows {
		validated := validateLateReturnRow(scope, refs, dorms, row)
		if len(validated.Errors) == 0 {
			key := validated.Record.DedupKey
			if previous, ok := seen[key]; ok {
				validated.Errors = append(validated.Errors, validationError(ErrorDuplicateInBatch, "inspection_time", fmt.Sprintf("第 %d 行与第 %d 行为同一学生同一次晚归", row.RowNo, previous), "每次晚归保留一条记录"))
			} else {
				seen[key] = row.RowNo
				keys = append(keys, key)
			}
		}
		result.Rows = append(result.Rows, validated)
	}
	existing, err := a.repository.FindExistingAttendanceKeys(ctx, scope, keys)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("find existing late-return events: %w", err)
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		if _, ok := existing[row.Record.DedupKey]; ok && row.Record.DedupKey != "" {
			row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "inspection_time", "该学生此时已有晚归记录", "核对已有记录，不重复导入"))
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}
func validateLateReturnRow(scope AttendanceImportScope, refs AttendanceReferences, dorms map[string]LateReturnDormitoryReference, row ParsedRow) InspectionValidationRow {
	result := InspectionValidationRow{RowNo: row.RowNo, Errors: []ValidationError{}}
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	for _, field := range []string{"student_no", "student_name", "class_name", "inspection_date", "inspection_time", "dormitory_name"} {
		if value(field) == "" {
			result.Errors = append(result.Errors, validationError(ErrorRequiredField, field, "必填字段不能为空", "按模板填写此字段"))
		}
	}
	student, class, issues := validateStudentImportReferences(scope, refs, row)
	result.Errors = append(result.Errors, issues...)
	dorm, ok := dorms[student.ID]
	if student.ID != "" && !ok {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "dormitory_name", "学生未分配有效宿舍", "核对学生住宿及宿舍区、楼栋、房间是否启用"))
	} else if ok && dorm.Name != value("dormitory_name") {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "dormitory_name", "宿舍与学生当前住宿不一致", "使用系统中的完整宿舍名称"))
	}
	if utf8.RuneCountInString(value("remark")) > 500 {
		result.Errors = append(result.Errors, validationError(ErrorTextTooLong, "remark", "备注不能超过500字", "缩短备注"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	raw, err := json.Marshal(inspection.LateReturnDetails{StudentID: student.ID, ClassID: class.ID, DormitoryID: dorm.ID, Date: value("inspection_date"), Time: value("inspection_time"), Remark: value("remark")})
	if err != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidWorkbook, "inspection_date", "无法生成检查明细", "重新核对模板"))
		return result
	}
	normalized, domainIssues := inspection.ValidateDetails(scope.TypeCode, inspection.DetailContext{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, WeekStart: scope.WeekStart.Format("2006-01-02"), WeekEnd: scope.WeekEnd.Format("2006-01-02")}, raw)
	for _, issue := range domainIssues {
		code, field, message := ErrorInvalidDate, "inspection_date", "晚归日期格式无效"
		if issue.Code == "OUTSIDE_WEEK" {
			code, message = ErrorOccurredOutsideWeek, "晚归日期不在当前周次内"
		} else if strings.Contains(issue.Path, "time") {
			code, field, message = ErrorInvalidTime, "inspection_time", "晚归时间格式无效"
		}
		result.Errors = append(result.Errors, validationError(code, field, message, "使用当前周次内日期和24小时制时间"))
	}
	if len(result.Errors) > 0 {
		return result
	}
	result.Record = InspectionImportRecord{SchemaVersion: 1, ID: uuid.NewString(), TypeCode: scope.TypeCode, Input: inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, Details: raw}, References: inspection.InspectionReferences{Classes: []inspection.ClassReference{{ID: class.ID, Name: class.Name, Grade: class.Grade}}, Students: []inspection.StudentReference{{ID: student.ID, ClassID: student.ClassID, Number: student.StudentNo, Name: student.Name}}}, CreatedBy: scope.CreatedBy, ImportBatchID: scope.ImportBatchID, DedupKey: normalized.DuplicateKey}
	return result
}
