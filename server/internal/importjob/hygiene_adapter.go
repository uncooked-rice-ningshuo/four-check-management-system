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

// HygieneNamedReference retains all exact-name matches so collisions never
// silently select a different object.
type HygieneNamedReference struct{ ID, Code, Name, ParentID string }

type HygieneImportLookup struct {
	TypeCode         string
	DormitoryNames   []string
	ClassroomNames   []string
	ResponsibleNames []string
}

type HygieneImportObjects struct {
	Dormitories map[string][]HygieneNamedReference
	Classrooms  map[string][]HygieneNamedReference
}

// HygieneImportRepository resolves authorized students/classes and checked objects.
type HygieneImportRepository interface {
	AttendanceReferenceRepository
	LoadHygieneImportObjects(context.Context, HygieneImportLookup) (HygieneImportObjects, error)
}

func validateHygieneImport(ctx context.Context, repository HygieneImportRepository, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	lookup := AttendanceReferenceLookup{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, ClassID: scope.ClassID, Grade: scope.Grade}
	objectsLookup := HygieneImportLookup{TypeCode: scope.TypeCode}
	for _, row := range rows {
		lookup.ClassNames = append(lookup.ClassNames, strings.TrimSpace(row.Values["responsible_class_names"]))
		lookup.StudentNos = append(lookup.StudentNos, strings.TrimSpace(row.Values["responsible_student_no"]))
		objectsLookup.DormitoryNames = append(objectsLookup.DormitoryNames, strings.TrimSpace(row.Values["dormitory_name"]))
		objectsLookup.ResponsibleNames = append(objectsLookup.ResponsibleNames, strings.TrimSpace(row.Values["responsible_dormitory_name"]))
		objectsLookup.ClassroomNames = append(objectsLookup.ClassroomNames, strings.TrimSpace(row.Values["classroom_name"]))
	}
	refs, err := repository.LoadAttendanceReferences(ctx, lookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load hygiene responsibility: %w", err)
	}
	objects, err := repository.LoadHygieneImportObjects(ctx, objectsLookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load hygiene objects: %w", err)
	}
	result := InspectionValidationResult{Rows: make([]InspectionValidationRow, 0, len(rows))}
	seen := map[string]int{}
	candidates := make([]InspectionDuplicateCandidate, 0, len(rows))
	for _, row := range rows {
		checked := validateHygieneRow(scope, refs, objects, row)
		if len(checked.Errors) == 0 {
			key := checked.Record.DedupKey
			if previous, ok := seen[key]; ok {
				checked.Errors = append(checked.Errors, validationError(ErrorDuplicateInBatch, "inspection_date", fmt.Sprintf("第 %d 行与第 %d 行为同一对象同一天", row.RowNo, previous), "每个检查对象每天只保留一条记录"))
			} else {
				seen[key] = row.RowNo
				if checked.Record.Duplicate != nil {
					candidates = append(candidates, *checked.Record.Duplicate)
				}
			}
		}
		result.Rows = append(result.Rows, checked)
	}
	existing, err := repository.FindExistingDailyInspectionDuplicates(ctx, scope, candidates)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("find existing hygiene dates: %w", err)
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		if _, found := existing[row.Record.DedupKey]; found && row.Record.DedupKey != "" {
			row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "inspection_date", "该对象当天已有卫生记录，不支持复查", "核对已有记录，不要重复录入"))
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}

func validateHygieneRow(scope AttendanceImportScope, refs AttendanceReferences, objects HygieneImportObjects, row ParsedRow) InspectionValidationRow {
	checked := InspectionValidationRow{RowNo: row.RowNo, Errors: []ValidationError{}}
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	issue := func(code ErrorCode, field, message, suggestion string) {
		checked.Errors = append(checked.Errors, validationError(code, field, message, suggestion))
	}
	objectField := "dormitory_name"
	required := []string{"inspection_date", "result", "responsible_class_names", "responsible_student_no"}
	switch scope.TypeCode {
	case inspection.InspectionTypeClassroomHygiene:
		objectField = "classroom_name"
		required = append(required, objectField)
	case inspection.InspectionTypePublicDormitoryHygiene:
		objectField = "responsible_dormitory_name"
		required = append(required, objectField)
	case inspection.InspectionTypeDormitoryHygiene:
		required = append(required, objectField)
	default:
		issue(ErrorCode("INVALID_INSPECTION_TYPE"), "result", "卫生检查类型无效", "选择对应的卫生导入模板")
	}
	for _, field := range required {
		if value(field) == "" {
			issue(ErrorRequiredField, field, "必填字段不能为空", "按模板填写此字段")
		}
	}
	if utf8.RuneCountInString(value("description")) > 1000 {
		issue(ErrorTextTooLong, "description", "情况说明不能超过1000字", "缩短情况说明")
	}
	if utf8.RuneCountInString(value("remark")) > 500 {
		issue(ErrorTextTooLong, "remark", "备注不能超过500字", "缩短备注")
	}
	resolve := func(values map[string][]HygieneNamedReference, name, field string) HygieneNamedReference {
		matches := values[name]
		if len(matches) == 0 {
			issue(ErrorReferenceNotFound, field, "未找到启用的对应名称", "使用系统中的完整名称，并检查是否启用")
			return HygieneNamedReference{}
		}
		if len(matches) > 1 {
			issue(ErrorReferenceMismatch, field, "名称对应多条基础数据，无法唯一确定", "请管理员消除重名后重新上传")
			return HygieneNamedReference{}
		}
		return matches[0]
	}
	var dormitory, classroom HygieneNamedReference
	switch scope.TypeCode {
	case inspection.InspectionTypeDormitoryHygiene:
		dormitory = resolve(objects.Dormitories, value("dormitory_name"), "dormitory_name")
	case inspection.InspectionTypeClassroomHygiene:
		classroom = resolve(objects.Classrooms, value("classroom_name"), "classroom_name")
	case inspection.InspectionTypePublicDormitoryHygiene:
		dormitory = resolve(objects.Dormitories, value("responsible_dormitory_name"), "responsible_dormitory_name")
	}

	className, studentNo := value("responsible_class_names"), value("responsible_student_no")
	student, studentOK := refs.StudentsByNo[studentNo]
	class, classOK := importClassForStudent(refs, className, student)
	if className != "" && !classOK {
		issue(ErrorReferenceNotFound, "responsible_class_names", "责任班级不存在、未启用或不在当前范围", "填写当前授权范围内的完整班级名称")
	}
	if studentNo != "" && !studentOK {
		issue(ErrorReferenceNotFound, "responsible_student_no", "责任学生不存在、未启用或不在当前范围", "核对学号并使用其当前班级")
	}
	if classOK && ((scope.ClassID != "" && scope.ClassID != class.ID) || (scope.Grade != "" && scope.Grade != class.Grade)) {
		issue(ErrorReferenceMismatch, "responsible_class_names", "责任班级超出当前导入范围", "使用当前有权录入的班级")
	}
	if studentOK && classOK && student.ClassID != class.ID {
		issue(ErrorReferenceMismatch, "responsible_student_no", "责任学生不属于所填班级", "核对学生当前所在班级")
	}
	// The name column is a readability aid: when filled it must match the number.
	if studentName := value("responsible_student_name"); studentOK && studentName != "" && studentName != student.Name {
		issue(ErrorReferenceMismatch, "responsible_student_name", "责任学生姓名与学号不一致", "使用学号对应的姓名")
	}
	resultValue := value("result")
	switch resultValue {
	case "优秀":
		resultValue = inspection.HygieneExcellent
	case "合格":
		resultValue = inspection.HygieneQualified
	case "不合格":
		resultValue = inspection.HygieneUnqualified
	}
	hygiene := inspection.HygieneDetails{
		Date: value("inspection_date"), Result: resultValue,
		ResponsibleClassIDs: []string{class.ID}, ResponsibleStudentIDs: []string{student.ID},
		Description: value("description"), Remark: value("remark"),
	}
	var details inspection.InspectionDetails
	var objectID string
	switch scope.TypeCode {
	case inspection.InspectionTypeDormitoryHygiene:
		objectID = dormitory.ID
		details = &inspection.DormitoryHygieneDetails{DormitoryID: dormitory.ID, HygieneDetails: hygiene}
	case inspection.InspectionTypeClassroomHygiene:
		objectID = classroom.ID
		details = &inspection.ClassroomHygieneDetails{ClassroomID: classroom.ID, HygieneDetails: hygiene}
	case inspection.InspectionTypePublicDormitoryHygiene:
		objectID = dormitory.ID
		details = &inspection.PublicDormitoryHygieneDetails{ResponsibleDormitoryIDs: []string{dormitory.ID}, HygieneDetails: hygiene}
	}
	if len(checked.Errors) > 0 {
		return checked
	}
	raw, err := json.Marshal(details)
	if err != nil {
		issue(ErrorInvalidWorkbook, "result", "无法生成检查明细", "核对模板后重试")
		return checked
	}
	context := inspection.DetailContext{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, WeekStart: scope.WeekStart.Format("2006-01-02"), WeekEnd: scope.WeekEnd.Format("2006-01-02")}
	normalized, problems := inspection.ValidateDetails(scope.TypeCode, context, raw)
	for _, problem := range problems {
		field := strings.TrimPrefix(problem.Path, "details.")
		if field == "date" {
			field = "inspection_date"
		} else if field == "responsibleClassIds" {
			field = "responsible_class_names"
		} else if field == "responsibleStudentIds" {
			field = "responsible_student_no"
		} else if field == "responsibleDormitoryIds" {
			field = "responsible_dormitory_name"
		}
		code, message, suggestion := ErrorCode("INVALID_HYGIENE_DETAIL"), "检查字段格式或组合不符合要求", "核对日期、结果和责任人信息"
		if problem.Code == "OUTSIDE_WEEK" {
			code, message = ErrorOccurredOutsideWeek, "检查日期不在当前周次内"
		} else if field == "description" {
			code, message, suggestion = ErrorRequiredField, "不合格结果必须填写情况说明", "填写具体情况，也可以在表单中使用说明短语起稿"
		}
		issue(code, field, message, suggestion)
	}
	if len(checked.Errors) > 0 {
		return checked
	}
	normalizedRaw, err := json.Marshal(normalized.Details)
	if err != nil {
		issue(ErrorInvalidWorkbook, "result", "无法规范化检查明细", "核对模板后重试")
		return checked
	}
	classes := []inspection.ClassReference{{ID: class.ID, Name: class.Name, Grade: class.Grade}}
	students := []inspection.StudentReference{{ID: student.ID, ClassID: student.ClassID, Number: student.StudentNo, Name: student.Name}}
	checked.Record = InspectionImportRecord{
		SchemaVersion: 1, ID: uuid.NewString(), TypeCode: scope.TypeCode,
		Input:      inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, AttributionMode: attributionModeForScope(scope), Details: normalizedRaw},
		References: inspection.InspectionReferences{Classes: classes, Students: students}, CreatedBy: scope.CreatedBy, ImportBatchID: scope.ImportBatchID,
		DedupKey:  normalized.DuplicateKey,
		Duplicate: &InspectionDuplicateCandidate{Key: normalized.DuplicateKey, ClassID: class.ID, StudentID: student.ID, ObjectID: objectID, Date: value("inspection_date")},
	}
	return checked
}
