package importjob

import (
	"context"
	"encoding/json"
	"fmt"
	"four-check-management-system/server/internal/inspection"
	"github.com/google/uuid"
	"sort"
	"strings"
)

// HygieneNamedReference retains all matches so display-name collisions never
// silently select a different object or dictionary value.
type HygieneNamedReference struct{ ID, Code, Name, ParentID string }

// HygieneImportLookup limits lookups to object and reason names in one batch.
type HygieneImportLookup struct {
	TypeCode                                               string
	DormitoryNames, ClassroomNames, AreaNames, ReasonNames []string
}

// HygieneImportObjects contains enabled objects, grouped by exact display name.
type HygieneImportObjects struct {
	Dormitories, Classrooms, Areas, Reasons map[string][]HygieneNamedReference
}

// HygieneImportRepository resolves scoped people separately from checked objects.
type HygieneImportRepository interface {
	AttendanceReferenceRepository
	LoadHygieneImportObjects(context.Context, HygieneImportLookup) (HygieneImportObjects, error)
	FindExistingHygieneKeys(context.Context, AttendanceImportScope, []string) (map[string]struct{}, error)
}

func validateHygieneImport(ctx context.Context, repository HygieneImportRepository, scope AttendanceImportScope, rows []ParsedRow) (InspectionValidationResult, error) {
	lookup := AttendanceReferenceLookup{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, ClassID: scope.ClassID, Grade: scope.Grade}
	objectsLookup := HygieneImportLookup{TypeCode: scope.TypeCode}
	for _, row := range rows {
		lookup.ClassNames = append(lookup.ClassNames, hygieneList(row.Values["responsible_class_names"])...)
		lookup.StudentNos = append(lookup.StudentNos, hygieneList(row.Values["responsible_student_nos"])...)
		objectsLookup.DormitoryNames = append(objectsLookup.DormitoryNames, strings.TrimSpace(row.Values["dormitory_name"]))
		objectsLookup.ClassroomNames = append(objectsLookup.ClassroomNames, strings.TrimSpace(row.Values["classroom_name"]))
		objectsLookup.AreaNames = append(objectsLookup.AreaNames, strings.TrimSpace(row.Values["area_name"]))
		objectsLookup.ReasonNames = append(objectsLookup.ReasonNames, strings.TrimSpace(row.Values["reason_name"]))
	}
	refs, err := repository.LoadAttendanceReferences(ctx, lookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load hygiene responsibilities: %w", err)
	}
	objects, err := repository.LoadHygieneImportObjects(ctx, objectsLookup)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("load hygiene objects: %w", err)
	}
	result := InspectionValidationResult{Rows: make([]InspectionValidationRow, 0, len(rows))}
	seen := map[string]int{}
	keys := []string{}
	for _, row := range rows {
		checked := validateHygieneRow(scope, refs, objects, row)
		if len(checked.Errors) == 0 {
			key := checked.Record.DedupKey
			if previous, ok := seen[key]; ok {
				checked.Errors = append(checked.Errors, validationError(ErrorDuplicateInBatch, "inspection_round", fmt.Sprintf("第 %d 行与第 %d 行为同一对象区域同一次检查", row.RowNo, previous), "每次检查保留一行，责任班级和学生写在同一行"))
			} else {
				seen[key] = row.RowNo
				keys = append(keys, key)
			}
		}
		result.Rows = append(result.Rows, checked)
	}
	existing, err := repository.FindExistingHygieneKeys(ctx, scope, keys)
	if err != nil {
		return InspectionValidationResult{}, fmt.Errorf("find existing hygiene checks: %w", err)
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		if _, ok := existing[row.Record.DedupKey]; ok && row.Record.DedupKey != "" {
			row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "inspection_round", "该对象区域此轮次或时间已有检查记录", "核对已有记录，不重复导入"))
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}

func hygieneList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(strings.ReplaceAll(raw, "；", ";"), ";")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func validateHygieneRow(scope AttendanceImportScope, refs AttendanceReferences, objects HygieneImportObjects, row ParsedRow) InspectionValidationRow {
	checked := InspectionValidationRow{RowNo: row.RowNo, Errors: []ValidationError{}}
	value := func(key string) string { return strings.TrimSpace(row.Values[key]) }
	issue := func(code ErrorCode, field, message, suggestion string) {
		checked.Errors = append(checked.Errors, validationError(code, field, message, suggestion))
	}
	required := []string{"inspection_date", "result", "responsible_class_names"}
	if scope.TypeCode == inspection.InspectionTypeClassroomHygiene {
		required = append(required, "classroom_name", "area_name")
	} else {
		required = append(required, "dormitory_name")
	}
	for _, field := range required {
		if value(field) == "" {
			issue(ErrorRequiredField, field, "必填字段不能为空", "按模板填写此字段")
		}
	}
	resolve := func(values map[string][]HygieneNamedReference, name, field string) HygieneNamedReference {
		matches := values[name]
		if len(matches) == 0 {
			issue(ErrorReferenceNotFound, field, "未找到启用的对应名称", "使用系统中的完整名称，并检查是否启用")
			return HygieneNamedReference{}
		}
		if len(matches) > 1 {
			issue(ErrorReferenceMismatch, field, "名称对应多条基础数据，无法唯一确定", "请管理员消除重名或名称拼接歧义后重新上传")
			return HygieneNamedReference{}
		}
		return matches[0]
	}
	var dorm, classroom, area HygieneNamedReference
	if scope.TypeCode == inspection.InspectionTypeClassroomHygiene {
		classroom = resolve(objects.Classrooms, value("classroom_name"), "classroom_name")
		// Areas with the same name in other classrooms are distinct valid objects.
		matching := map[string][]HygieneNamedReference{}
		for _, candidate := range objects.Areas[value("area_name")] {
			if candidate.ParentID == classroom.ID && classroom.ID != "" {
				matching[value("area_name")] = append(matching[value("area_name")], candidate)
			}
		}
		area = resolve(matching, value("area_name"), "area_name")
	} else {
		dorm = resolve(objects.Dormitories, value("dormitory_name"), "dormitory_name")
	}
	reason := HygieneNamedReference{}
	if value("reason_name") != "" {
		reason = resolve(objects.Reasons, value("reason_name"), "reason_name")
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
	hygiene := inspection.HygieneDetails{Date: value("inspection_date"), Time: value("inspection_time"), Round: value("inspection_round"), Result: resultValue, ReasonCode: reason.Code, Description: value("description"), Remark: value("remark"), ResponsibleClassIDs: []string{}}
	snapshots := inspection.InspectionReferences{Classes: []inspection.ClassReference{}, Students: []inspection.StudentReference{}}
	classIDs := map[string]bool{}
	for _, name := range hygieneList(value("responsible_class_names")) {
		class, ok := refs.ClassesByName[name]
		if !ok {
			issue(ErrorReferenceNotFound, "responsible_class_names", "责任班级不存在、未启用或不在当前范围", "填写当前授权范围内的完整班级名称")
			continue
		}
		if (scope.ClassID != "" && scope.ClassID != class.ID) || (scope.Grade != "" && scope.Grade != class.Grade) {
			issue(ErrorReferenceMismatch, "responsible_class_names", "责任班级超出当前导入范围", "所有责任班级均须在当前范围内")
			continue
		}
		hygiene.ResponsibleClassIDs = append(hygiene.ResponsibleClassIDs, class.ID)
		classIDs[class.ID] = true
		snapshots.Classes = append(snapshots.Classes, inspection.ClassReference{ID: class.ID, Name: class.Name, Grade: class.Grade})
	}
	for _, number := range hygieneList(value("responsible_student_nos")) {
		student, ok := refs.StudentsByNo[number]
		if !ok || !classIDs[student.ClassID] {
			issue(ErrorReferenceNotFound, "responsible_student_nos", "责任学生不存在、未启用或不属于所填责任班级", "核对学号并将其当前班级列入责任班级")
			continue
		}
		hygiene.ResponsibleStudentIDs = append(hygiene.ResponsibleStudentIDs, student.ID)
		snapshots.Students = append(snapshots.Students, inspection.StudentReference{ID: student.ID, ClassID: student.ClassID, Number: student.StudentNo, Name: student.Name})
	}
	if len(checked.Errors) > 0 {
		return checked
	}
	var details inspection.InspectionDetails = &inspection.DormitoryHygieneDetails{DormitoryID: dorm.ID, HygieneDetails: hygiene}
	if scope.TypeCode == inspection.InspectionTypeClassroomHygiene {
		details = &inspection.ClassroomHygieneDetails{ClassroomID: classroom.ID, AreaID: area.ID, HygieneDetails: hygiene}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		issue(ErrorInvalidWorkbook, "result", "无法生成检查明细", "核对模板后重试")
		return checked
	}
	normalized, issues := inspection.ValidateDetails(scope.TypeCode, inspection.DetailContext{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, WeekStart: scope.WeekStart.Format("2006-01-02"), WeekEnd: scope.WeekEnd.Format("2006-01-02")}, raw)
	fields := map[string]string{"date": "inspection_date", "time": "inspection_time", "round": "inspection_round", "reasonCode": "reason_name", "description": "description", "responsibleClassIds": "responsible_class_names", "responsibleStudentIds": "responsible_student_nos"}
	for _, problem := range issues {
		field := strings.TrimPrefix(problem.Path, "details.")
		if mapped, ok := fields[field]; ok {
			field = mapped
		}
		code := ErrorCode("INVALID_HYGIENE_DETAIL")
		message := "检查字段格式或组合不符合要求"
		suggestion := "核对日期、结果、文本长度及责任列表，责任项不得重复"
		if problem.Code == "OUTSIDE_WEEK" {
			code = ErrorOccurredOutsideWeek
			message = "检查日期不在当前周次内"
		}
		if problem.Code == "OCCURRENCE_REQUIRED" || problem.Code == "OCCURRENCE_CONFLICT" {
			message = "检查时间与轮次必须且只能填写一项"
			suggestion = "填写24小时制时间或本次检查轮次"
		}
		if field == "reason_name" {
			message = "不合格结果必须填写原因"
			suggestion = "填写当前检查类型已启用的原因名称"
		}
		if problem.Code == "OTHER_REASON_REQUIRED" {
			message = "选择“其他”时必须填写具体原因"
			suggestion = "在情况说明列填写具体不合格原因"
		}
		issue(code, field, message, suggestion)
	}
	if len(checked.Errors) > 0 {
		return checked
	}
	raw, err = json.Marshal(normalized.Details)
	if err != nil {
		issue(ErrorInvalidWorkbook, "result", "无法生成检查明细", "核对模板后重试")
		return checked
	}
	sort.Slice(snapshots.Classes, func(i, j int) bool { return snapshots.Classes[i].ID < snapshots.Classes[j].ID })
	sort.Slice(snapshots.Students, func(i, j int) bool { return snapshots.Students[i].ID < snapshots.Students[j].ID })
	checked.Record = InspectionImportRecord{SchemaVersion: 1, ID: uuid.NewString(), TypeCode: scope.TypeCode, Input: inspection.InspectionInput{SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, Details: raw}, References: snapshots, CreatedBy: scope.CreatedBy, ImportBatchID: scope.ImportBatchID, DedupKey: normalized.DuplicateKey}
	return checked
}
