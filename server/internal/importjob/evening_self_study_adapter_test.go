package importjob

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/inspection"
	"reflect"
	"testing"
)

func eveningSelfStudyScope() AttendanceImportScope {
	scope := attendanceImportScope()
	scope.TypeCode = inspection.InspectionTypeEveningSelfStudy
	scope.InspectionTypeID = "30000000-0000-0000-0000-000000000008"
	scope.SemesterID = "b0000000-0000-0000-0000-000000000002"
	scope.WeekID = "b0000000-0000-0000-0000-000000000003"
	scope.DepartmentID = "10000000-0000-0000-0000-000000000003"
	scope.ImportBatchID = "b0000000-0000-0000-0000-000000000004"
	scope.CreatedBy = "b0000000-0000-0000-0000-000000000005"
	return scope
}

func eveningSelfStudyRow(rowNo int) ParsedRow {
	return ParsedRow{RowNo: rowNo, Values: map[string]string{
		"student_no": "S20260001", "student_name": "测试学生甲", "class_name": "示例班级A",
		"classroom_code": "A101", "absence_date": "2026-06-17", "absence_time": "20:15", "remark": "晚自习缺勤",
	}}
}

func eveningSelfStudyReferences() AttendanceReferences {
	references := validAttendanceReferences()
	references.StudentsByNo["S20260001"] = AttendanceStudentReference{
		ID: "c1000000-0000-0000-0000-000000000001", StudentNo: "S20260001", Name: "测试学生甲",
		ClassID: "c1000000-0000-0000-0000-000000000002", ClassName: "示例班级A", Grade: "2026",
	}
	references.ClassesByName["示例班级A"] = AttendanceClassReference{ID: "c1000000-0000-0000-0000-000000000002", Name: "示例班级A", Grade: "2026"}
	references.ClassroomsByCode["A101"] = AttendanceClassroomReference{ID: "c1000000-0000-0000-0000-000000000003", Code: "A101"}
	return references
}

func TestEveningSelfStudyAdapterCreatesNoCourseAbsenceAndDetectsDuplicates(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{references: eveningSelfStudyReferences(), existing: map[string]struct{}{}}
	rows := []ParsedRow{eveningSelfStudyRow(5), eveningSelfStudyRow(6)}
	result, err := NewEveningSelfStudyAdapter(repository).Validate(context.Background(), eveningSelfStudyScope(), rows)
	if err != nil || result.ValidCount != 1 || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatalf("preview = %#v, %v", result, err)
	}
	if repository.loadCalls != 1 || repository.existingCalls != 1 || repository.lookup.DepartmentID != eveningSelfStudyScope().DepartmentID ||
		!reflect.DeepEqual(repository.lookup.StudentNos, []string{"S20260001"}) || len(repository.lookup.CourseNames) != 0 || len(repository.lookup.CourseIDs) != 0 {
		t.Fatalf("references were not bulk loaded within the scoped, no-course lookup: %#v", repository)
	}
	if repository.lookup.ClassroomCodes[0] != "A101" {
		t.Fatalf("classroom lookup missing: %#v", repository.lookup)
	}
	record := result.Rows[0].Record
	if record.TypeCode != inspection.InspectionTypeEveningSelfStudy || len(record.References.Classes) != 1 || len(record.References.Students) != 1 ||
		record.References.Classes[0].ID != "c1000000-0000-0000-0000-000000000002" || record.References.Students[0].ID != "c1000000-0000-0000-0000-000000000001" || record.DedupKey == "" {
		t.Fatalf("normalized record = %#v", record)
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(record.Input.Details, &input); err != nil {
		t.Fatal(err)
	}
	for _, serverOwned := range []string{"period", "absenceHours", "penaltyMultiplier", "penaltyHours"} {
		if _, found := input[serverOwned]; found {
			t.Fatalf("import input must leave %q to the server: %s", serverOwned, record.Input.Details)
		}
	}
	if _, found := input["courseId"]; found {
		t.Fatalf("evening self-study must not bind a course: %s", record.Input.Details)
	}
	repository.existing[record.DedupKey] = struct{}{}
	result, err = NewEveningSelfStudyAdapter(repository).Validate(context.Background(), eveningSelfStudyScope(), []ParsedRow{eveningSelfStudyRow(5)})
	if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatalf("existing record accepted: %#v, %v", result, err)
	}
}

func TestEveningSelfStudyAdapterRejectsInvalidRowsAndScope(t *testing.T) {
	for _, test := range []struct {
		field, value string
		code         ErrorCode
	}{
		{"student_no", "", ErrorRequiredField}, {"student_no", "UNKNOWN", ErrorReferenceNotFound},
		{"student_name", "错误姓名", ErrorReferenceMismatch}, {"class_name", "其他班", ErrorReferenceNotFound},
		{"classroom_code", "B999", ErrorReferenceNotFound}, {"absence_date", "2026-06-22", ErrorOccurredOutsideWeek},
		{"absence_time", "25:00", ErrorInvalidTime}, {"remark", string(make([]rune, 501)), ErrorTextTooLong},
	} {
		t.Run(test.field, func(t *testing.T) {
			row := eveningSelfStudyRow(5)
			row.Values[test.field] = test.value
			result, err := NewEveningSelfStudyAdapter(&fakeAttendanceReferenceRepository{references: eveningSelfStudyReferences()}).Validate(context.Background(), eveningSelfStudyScope(), []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, test.code) {
				t.Fatalf("invalid %s accepted: %#v, %v", test.field, result, err)
			}
		})
	}
	for _, scope := range []AttendanceImportScope{
		func() AttendanceImportScope { s := eveningSelfStudyScope(); s.ClassID = "other-class"; return s }(),
		func() AttendanceImportScope { s := eveningSelfStudyScope(); s.Grade = "2025"; return s }(),
	} {
		result, err := NewEveningSelfStudyAdapter(&fakeAttendanceReferenceRepository{references: eveningSelfStudyReferences()}).Validate(context.Background(), scope, []ParsedRow{eveningSelfStudyRow(5)})
		if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorReferenceMismatch) {
			t.Fatalf("scope escaped: %#v, %v", result, err)
		}
	}
}

func TestEveningSelfStudyAdapterRequiresConfiguredMultiplierAndPropagatesFailures(t *testing.T) {
	references := validAttendanceReferences()
	references.WeekPenaltyMultiplier = nil
	repository := &fakeAttendanceReferenceRepository{references: references}
	_, err := NewEveningSelfStudyAdapter(repository).Validate(context.Background(), eveningSelfStudyScope(), []ParsedRow{eveningSelfStudyRow(5)})
	if !errors.Is(err, inspection.ErrAttendancePenaltyUnconfigured) || repository.existingCalls != 0 {
		t.Fatalf("unconfigured week = %v, duplicate lookups = %d", err, repository.existingCalls)
	}
	failure := errors.New("isolated repository failure")
	for _, repository := range []*fakeAttendanceReferenceRepository{{loadErr: failure}, {references: eveningSelfStudyReferences(), existingErr: failure}} {
		if _, err := NewEveningSelfStudyAdapter(repository).Validate(context.Background(), eveningSelfStudyScope(), []ParsedRow{eveningSelfStudyRow(5)}); !errors.Is(err, failure) {
			t.Fatalf("infrastructure error became a successful preview: %v", err)
		}
	}
	wrongType := eveningSelfStudyScope()
	wrongType.TypeCode = inspection.InspectionTypeMorning
	if _, err := NewEveningSelfStudyAdapter(&fakeAttendanceReferenceRepository{}).Validate(context.Background(), wrongType, nil); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("wrong type accepted: %v", err)
	}
}
