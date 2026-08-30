package importjob

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAttendanceReferenceRepository struct {
	references    AttendanceReferences
	existing      map[string]struct{}
	loadErr       error
	existingErr   error
	loadCalls     int
	existingCalls int
	lookup        AttendanceReferenceLookup
}

func (f *fakeAttendanceReferenceRepository) LoadAttendanceReferences(_ context.Context, lookup AttendanceReferenceLookup) (AttendanceReferences, error) {
	f.loadCalls++
	f.lookup = lookup
	return f.references, f.loadErr
}

func (f *fakeAttendanceReferenceRepository) FindExistingAttendanceKeys(_ context.Context, scope AttendanceImportScope, keys []string) (map[string]struct{}, error) {
	f.existingCalls++
	return f.existing, f.existingErr
}

func validAttendanceReferences() AttendanceReferences {
	return AttendanceReferences{
		StudentsByNo: map[string]AttendanceStudentReference{
			"S20260001": {ID: "student-1", StudentNo: "S20260001", Name: "测试学生甲", ClassID: "class-1", ClassName: "示例班级A", Grade: "2026"},
			"S20260002": {ID: "student-2", StudentNo: "S20260002", Name: "测试学生乙", ClassID: "class-1", ClassName: "示例班级A", Grade: "2026"},
		},
		ClassesByName:         map[string]AttendanceClassReference{"示例班级A": {ID: "class-1", Name: "示例班级A", Grade: "2026"}},
		CoursesByClassAndName: map[string]AttendanceCourseReference{attendanceCourseKey("class-1", "数据结构"): {ID: "course-1", Name: "数据结构", ClassID: "class-1", ClassroomID: "room-1"}},
		ClassroomsByCode:      map[string]AttendanceClassroomReference{"A101": {ID: "room-1", Code: "A101"}},
	}
}

func attendanceImportScope() AttendanceImportScope {
	location := time.FixedZone("CST", 8*60*60)
	return AttendanceImportScope{
		InspectionTypeID: "inspection-attendance", SemesterID: "semester-1", WeekID: "week-1",
		DepartmentID: "department-study", ImportBatchID: "batch-1", CreatedBy: "account-1",
		WeekStart: time.Date(2026, 6, 15, 0, 0, 0, 0, location), WeekEnd: time.Date(2026, 6, 21, 0, 0, 0, 0, location),
		Location: location,
	}
}

func attendanceParsedRow(rowNo int, studentNo string) ParsedRow {
	return ParsedRow{RowNo: rowNo, Values: map[string]string{
		"student_no": studentNo, "student_name": "测试学生甲", "class_name": "示例班级A",
		"course_name": "数据结构", "absence_date": "2026-06-17", "absence_time": "08:00",
		"period": "第1-2节", "absence_hours": "2", "classroom_code": "A101", "remark": "示例数据",
	}}
}

func TestAttendanceAdapterNormalizesValidRowsWithOneBulkPrefetch(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{references: validAttendanceReferences(), existing: map[string]struct{}{}}
	adapter := NewAttendanceAdapter(repository)
	rows := []ParsedRow{attendanceParsedRow(5, "S20260001"), attendanceParsedRow(6, "S20260002")}
	rows[1].Values["student_name"] = "测试学生乙"

	result, err := adapter.Validate(context.Background(), attendanceImportScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	if repository.loadCalls != 1 || repository.existingCalls != 1 {
		t.Fatalf("bulk calls = references %d, existing %d", repository.loadCalls, repository.existingCalls)
	}
	if repository.lookup.DepartmentID != "department-study" || len(repository.lookup.StudentNos) != 2 || len(repository.lookup.CourseNames) != 1 {
		t.Fatalf("scoped lookup = %#v", repository.lookup)
	}
	if len(result.Rows) != 2 || len(result.Rows[0].Errors) != 0 || result.ValidCount != 2 || result.InvalidCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	record := result.Rows[0].Record
	if record.StudentID != "student-1" || record.CourseID != "course-1" || record.ClassroomID != "room-1" || record.AbsenceHours != 2 {
		t.Fatalf("record = %#v", record)
	}
	if record.OccurredAt.Format(time.RFC3339) != "2026-06-17T08:00:00+08:00" || record.DedupKey == "" {
		t.Fatalf("normalized occurrence/key = %s / %q", record.OccurredAt.Format(time.RFC3339), record.DedupKey)
	}
}

func TestAttendanceAdapterReportsStableSafeFieldErrors(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{references: validAttendanceReferences(), existing: map[string]struct{}{}}
	adapter := NewAttendanceAdapter(repository)
	rows := []ParsedRow{
		attendanceParsedRow(5, "UNKNOWN"),
		attendanceParsedRow(6, "S20260001"),
		attendanceParsedRow(7, "S20260001"),
		attendanceParsedRow(8, "S20260001"),
	}
	rows[1].Values["student_name"] = "错误姓名"
	rows[2].Values["absence_time"] = "25:00"
	rows[2].Values["absence_hours"] = "0"
	rows[2].Values["remark"] = string(make([]rune, 501))
	rows[3].Values["absence_date"] = "2026-06-30"

	result, err := adapter.Validate(context.Background(), attendanceImportScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	want := map[ErrorCode]bool{
		ErrorReferenceNotFound: false, ErrorReferenceMismatch: false, ErrorOccurredOutsideWeek: false,
		ErrorInvalidTime: false, ErrorInvalidHours: false, ErrorTextTooLong: false,
	}
	for _, row := range result.Rows {
		for _, item := range row.Errors {
			if _, ok := want[item.Code]; ok {
				want[item.Code] = true
			}
			if item.Field == "" || item.Message == "" || item.Suggestion == "" {
				t.Fatalf("incomplete error = %#v", item)
			}
			if item.Message == repositoryFailureText || item.Suggestion == repositoryFailureText {
				t.Fatalf("error leaks repository detail: %#v", item)
			}
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing error code %s in %#v", code, result.Rows)
		}
	}
}

func TestAttendanceAdapterDetectsBatchAndExistingDuplicates(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{references: validAttendanceReferences(), existing: map[string]struct{}{}}
	adapter := NewAttendanceAdapter(repository)
	rows := []ParsedRow{attendanceParsedRow(5, "S20260001"), attendanceParsedRow(6, "S20260001")}
	first, err := adapter.Validate(context.Background(), attendanceImportScope(), rows[:1])
	if err != nil || len(first.Rows[0].Errors) != 0 {
		t.Fatalf("first validation = %#v, %v", first, err)
	}
	repository.existing[first.Rows[0].Record.DedupKey] = struct{}{}

	result, err := adapter.Validate(context.Background(), attendanceImportScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	if !hasValidationCode(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatalf("existing duplicate errors = %#v", result.Rows[0].Errors)
	}
	if !hasValidationCode(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatalf("batch duplicate errors = %#v", result.Rows[1].Errors)
	}
}

func TestAttendanceAdapterDisambiguatesSameCourseNameByClass(t *testing.T) {
	references := validAttendanceReferences()
	references.CoursesByClassAndName = map[string]AttendanceCourseReference{
		attendanceCourseKey("class-1", "数据结构"): {ID: "course-class-1", Name: "数据结构", ClassID: "class-1", ClassroomID: "room-1"},
		attendanceCourseKey("class-2", "数据结构"): {ID: "course-class-2", Name: "数据结构", ClassID: "class-2", ClassroomID: "room-2"},
	}
	repository := &fakeAttendanceReferenceRepository{references: references, existing: map[string]struct{}{}}
	result, err := NewAttendanceAdapter(repository).Validate(context.Background(), attendanceImportScope(), []ParsedRow{attendanceParsedRow(5, "S20260001")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows[0].Errors) != 0 || result.Rows[0].Record.CourseID != "course-class-1" {
		t.Fatalf("course resolution = %#v", result.Rows[0])
	}
}

func TestAttendanceAdapterRejectsRowsOutsideClassAndGradeScope(t *testing.T) {
	scope := attendanceImportScope()
	scope.ClassID, scope.Grade = "other-class", "2025"
	result, err := NewAttendanceAdapter(&fakeAttendanceReferenceRepository{references: validAttendanceReferences(), existing: map[string]struct{}{}}).Validate(context.Background(), scope, []ParsedRow{attendanceParsedRow(5, "S20260001")})
	if err != nil {
		t.Fatal(err)
	}
	if !hasValidationCode(result.Rows[0].Errors, ErrorReferenceMismatch) {
		t.Fatalf("scope errors = %#v", result.Rows[0].Errors)
	}
}

func TestAttendanceAdapterDefaultsBusinessTimeToChinaStandardTime(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })
	scope := attendanceImportScope()
	scope.Location = nil
	repository := &fakeAttendanceReferenceRepository{references: validAttendanceReferences(), existing: map[string]struct{}{}}
	result, err := NewAttendanceAdapter(repository).Validate(context.Background(), scope, []ParsedRow{attendanceParsedRow(5, "S20260001")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows[0].Errors) != 0 || result.Rows[0].Record.OccurredAt.Format(time.RFC3339) != "2026-06-17T08:00:00+08:00" {
		t.Fatalf("default business occurrence = %#v", result.Rows[0])
	}
}

const repositoryFailureText = "pq: relation students does not exist"

func TestAttendanceAdapterPropagatesRepositoryFailureWithoutTurningItIntoRowDetails(t *testing.T) {
	repository := &fakeAttendanceReferenceRepository{loadErr: errors.New(repositoryFailureText)}
	_, err := NewAttendanceAdapter(repository).Validate(context.Background(), attendanceImportScope(), []ParsedRow{attendanceParsedRow(5, "S20260001")})
	if err == nil || err.Error() == "" {
		t.Fatal("repository failure was not propagated")
	}
}

func hasValidationCode(errors []ValidationError, code ErrorCode) bool {
	for _, item := range errors {
		if item.Code == code {
			return true
		}
	}
	return false
}
