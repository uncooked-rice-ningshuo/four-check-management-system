package importjob

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/inspection"
	"testing"
)

type lateReturnReferences struct {
	fakeAttendanceReferenceRepository
	dorms   map[string]LateReturnDormitoryReference
	dormErr error
}

func (r *lateReturnReferences) LoadLateReturnDormitories(context.Context, AttendanceReferenceLookup) (map[string]LateReturnDormitoryReference, error) {
	return r.dorms, r.dormErr
}
func lateReturnScope() AttendanceImportScope {
	s := morningScope()
	s.TypeCode = inspection.InspectionTypeLateReturn
	return s
}
func lateReturnRow(n int) ParsedRow {
	r := morningRow(n)
	r.Values["inspection_time"] = "23:05"
	r.Values["dormitory_name"] = "示例区 示例楼 101"
	return r
}
func lateReturnRepository() *lateReturnReferences {
	return &lateReturnReferences{fakeAttendanceReferenceRepository: fakeAttendanceReferenceRepository{references: morningRefs()}, dorms: map[string]LateReturnDormitoryReference{"c0000000-0000-0000-0000-000000000001": {ID: "c0000000-0000-0000-0000-000000000003", Name: "示例区 示例楼 101"}}}
}
func TestLateReturnAdapterUsesStudentEventAndCurrentDormitory(t *testing.T) {
	repo := lateReturnRepository()
	adapter := NewLateReturnAdapter(repo)
	result, err := adapter.Validate(context.Background(), lateReturnScope(), []ParsedRow{lateReturnRow(5), lateReturnRow(6)})
	if err != nil || result.ValidCount != 1 || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatalf("%+v %v", result, err)
	}
	r := result.Rows[0].Record
	parsed, issues := inspection.ValidateDetails(inspection.InspectionTypeLateReturn, inspection.DetailContext{SemesterID: r.Input.SemesterID, WeekID: r.Input.WeekID, InspectionTypeID: r.Input.InspectionTypeID, WeekStart: "2026-10-01", WeekEnd: "2026-10-07"}, r.Input.Details)
	if len(issues) != 0 || parsed.ViolationCount != 1 || parsed.OccurredAt.Format("15:04:05") != "23:05:00" {
		t.Fatal(parsed, issues)
	}
	repo.existing = map[string]struct{}{r.DedupKey: {}}
	result, err = adapter.Validate(context.Background(), lateReturnScope(), []ParsedRow{lateReturnRow(5)})
	if err != nil || !hasMorningError(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatal(result, err)
	}
}
func TestLateReturnAdapterRejectsInvalidTimeScopeAndDormitory(t *testing.T) {
	for _, test := range []struct {
		field, value string
		code         ErrorCode
	}{
		{"inspection_time", "", ErrorRequiredField}, {"inspection_time", "25:00", ErrorInvalidTime}, {"inspection_date", "2026-10-08", ErrorOccurredOutsideWeek}, {"dormitory_name", "另一区 示例楼 101", ErrorReferenceMismatch}, {"student_name", "他人", ErrorReferenceMismatch}, {"student_no", "unknown", ErrorReferenceNotFound},
	} {
		t.Run(test.field+test.value, func(t *testing.T) {
			row := lateReturnRow(5)
			row.Values[test.field] = test.value
			result, err := NewLateReturnAdapter(lateReturnRepository()).Validate(context.Background(), lateReturnScope(), []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, test.code) {
				t.Fatal(result, err)
			}
		})
	}
	repo := lateReturnRepository()
	repo.dorms = map[string]LateReturnDormitoryReference{}
	result, err := NewLateReturnAdapter(repo).Validate(context.Background(), lateReturnScope(), []ParsedRow{lateReturnRow(5)})
	if err != nil || !hasMorningError(result.Rows[0].Errors, ErrorReferenceNotFound) {
		t.Fatal(result, err)
	}
	repo = lateReturnRepository()
	repo.dormErr = errors.New("unavailable")
	if _, err = NewLateReturnAdapter(repo).Validate(context.Background(), lateReturnScope(), []ParsedRow{lateReturnRow(5)}); !errors.Is(err, repo.dormErr) {
		t.Fatal(err)
	}
	scope := lateReturnScope()
	scope.ClassID = "c0000000-0000-0000-0000-000000000099"
	result, err = NewLateReturnAdapter(lateReturnRepository()).Validate(context.Background(), scope, []ParsedRow{lateReturnRow(5)})
	if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorReferenceMismatch) {
		t.Fatal(result, err)
	}
}
