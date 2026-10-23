package importjob

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/inspection"
	"strings"
	"testing"
	"time"
)

func morningScope() AttendanceImportScope {
	return AttendanceImportScope{TypeCode: inspection.InspectionTypeMorning, InspectionTypeID: "30000000-0000-0000-0000-000000000003", SemesterID: "b0000000-0000-0000-0000-000000000002", WeekID: "b0000000-0000-0000-0000-000000000003", DepartmentID: "10000000-0000-0000-0000-000000000003", ImportBatchID: "b0000000-0000-0000-0000-000000000004", CreatedBy: "b0000000-0000-0000-0000-000000000005", WeekStart: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), WeekEnd: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)}
}
func morningRefs() AttendanceReferences {
	return AttendanceReferences{StudentsByNo: map[string]AttendanceStudentReference{"000001": {ID: "c0000000-0000-0000-0000-000000000001", StudentNo: "000001", Name: "虚构学生", ClassID: "c0000000-0000-0000-0000-000000000002", ClassName: "示例班", Grade: "2026级"}}, ClassesByName: map[string]AttendanceClassReference{"示例班": {ID: "c0000000-0000-0000-0000-000000000002", Name: "示例班", Grade: "2026级"}}}
}
func morningRow(n int) ParsedRow {
	return ParsedRow{RowNo: n, Values: map[string]string{"student_no": "000001", "student_name": "虚构学生", "class_name": "示例班", "inspection_date": "2026-10-01", "remark": "单次缺勤"}}
}

func TestMorningAdapterNormalizesOneStudentDayAndDetectsDuplicates(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{references: morningRefs()}
	adapter := NewMorningAdapter(repository)
	result, err := adapter.Validate(context.Background(), morningScope(), []ParsedRow{morningRow(5), morningRow(6)})
	if err != nil || result.ValidCount != 1 || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatalf("%+v %v", result, err)
	}
	normalized := result.Rows[0].Record
	var details inspection.MorningAttendanceDetails
	if err = json.Unmarshal(normalized.Input.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details.Date != "2026-10-01" || details.AbsenceCount != 1 || normalized.TypeCode != inspection.InspectionTypeMorning || normalized.References.Students[0].Number != "000001" {
		t.Fatal("invalid day record", normalized, details)
	}
	if strings.Contains(string(normalized.Input.Details), "time") || strings.Contains(string(normalized.Input.Details), "activityKind") {
		t.Fatal("invented time or subtype")
	}
	if repository.loadCalls != 1 || repository.existingCalls != 1 || repository.lookup.DepartmentID != morningScope().DepartmentID {
		t.Fatal("unscoped or repeated reference lookup")
	}
	repository.existing = map[string]struct{}{normalized.DedupKey: {}}
	result, err = adapter.Validate(context.Background(), morningScope(), []ParsedRow{morningRow(5)})
	if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatal("existing day accepted", result, err)
	}
}
func hasMorningError(values []ValidationError, code ErrorCode) bool {
	for _, v := range values {
		if v.Code == code {
			return true
		}
	}
	return false
}

func TestMorningAdapterRejectsInvalidReferencesAndFields(t *testing.T) {
	for _, test := range []struct {
		field, value string
		code         ErrorCode
	}{
		{"student_no", "", ErrorRequiredField}, {"student_no", "missing", ErrorReferenceNotFound}, {"student_name", "不匹配", ErrorReferenceMismatch}, {"class_name", "其他班", ErrorReferenceNotFound}, {"inspection_date", "2026-02-30", ErrorInvalidDate}, {"inspection_date", "2026-10-08", ErrorOccurredOutsideWeek}, {"remark", strings.Repeat("字", 501), ErrorTextTooLong},
	} {
		t.Run(test.field+test.value[:min(len(test.value), 12)], func(t *testing.T) {
			row := morningRow(5)
			row.Values[test.field] = test.value
			result, err := NewMorningAdapter(&fakeAttendanceReferenceRepository{references: morningRefs()}).Validate(context.Background(), morningScope(), []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, test.code) {
				t.Fatal("accepted invalid field", result, err)
			}
		})
	}
	for _, scope := range []AttendanceImportScope{func() AttendanceImportScope {
		s := morningScope()
		s.ClassID = "c0000000-0000-0000-0000-000000000099"
		return s
	}(), func() AttendanceImportScope { s := morningScope(); s.Grade = "2025级"; return s }()} {
		result, err := NewMorningAdapter(&fakeAttendanceReferenceRepository{references: morningRefs()}).Validate(context.Background(), scope, []ParsedRow{morningRow(5)})
		if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorReferenceMismatch) {
			t.Fatal("scope escaped", result, err)
		}
	}
}
func TestMorningAdapterPropagatesInfrastructureFailure(t *testing.T) {
	failed := errors.New("isolated failure")
	for _, repository := range []*fakeAttendanceReferenceRepository{{loadErr: failed}, {references: morningRefs(), existingErr: failed}} {
		if _, err := NewMorningAdapter(repository).Validate(context.Background(), morningScope(), []ParsedRow{morningRow(5)}); !errors.Is(err, failed) {
			t.Fatal("failure became valid empty preview", err)
		}
	}
}
