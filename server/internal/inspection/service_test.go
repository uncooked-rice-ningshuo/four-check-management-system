package inspection

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingRepository struct {
	query AttendanceListQuery
	page  AttendancePage
	err   error
}

func (r *recordingRepository) ListAttendance(_ context.Context, query AttendanceListQuery) (AttendancePage, error) {
	r.query = query
	return r.page, r.err
}

func validAttendanceInput() AttendanceInput {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return AttendanceInput{
		InspectionTypeID:    "inspection-attendance",
		InspectionTypeCode:  InspectionTypeAttendance,
		SemesterID:          "semester-1",
		WeekID:              "week-3",
		DepartmentID:        "department-study",
		StudentID:           "student-1",
		StudentNoSnapshot:   "2023120101",
		StudentNameSnapshot: "张晨",
		ClassID:             "class-1",
		ClassNameSnapshot:   "计科2301",
		GradeSnapshot:       "2023",
		CourseID:            "course-1",
		OccurredAt:          time.Date(2026, 6, 17, 8, 0, 0, 0, location),
		WeekStart:           time.Date(2026, 6, 15, 0, 0, 0, 0, location),
		WeekEnd:             time.Date(2026, 6, 21, 23, 59, 59, 0, location),
		AbsenceHours:        2,
		PeriodText:          "第1-2节",
		CreatedBy:           "account-1",
	}
}

func TestNewAttendanceRecordRejectsMissingContext(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AttendanceInput)
	}{
		{"inspection type", func(input *AttendanceInput) { input.InspectionTypeID = "" }},
		{"semester", func(input *AttendanceInput) { input.SemesterID = "" }},
		{"week", func(input *AttendanceInput) { input.WeekID = "" }},
		{"department", func(input *AttendanceInput) { input.DepartmentID = "" }},
		{"student", func(input *AttendanceInput) { input.StudentID = "" }},
		{"class", func(input *AttendanceInput) { input.ClassID = "" }},
		{"course", func(input *AttendanceInput) { input.CourseID = "" }},
		{"creator", func(input *AttendanceInput) { input.CreatedBy = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validAttendanceInput()
			test.mutate(&input)
			if _, err := NewAttendanceRecord(input); !errors.Is(err, ErrInvalidAttendanceRecord) {
				t.Fatalf("error = %v, want ErrInvalidAttendanceRecord", err)
			}
		})
	}
}

func TestNewAttendanceRecordRejectsNonPositiveAbsenceHours(t *testing.T) {
	input := validAttendanceInput()
	input.AbsenceHours = 0
	if _, err := NewAttendanceRecord(input); !errors.Is(err, ErrInvalidAttendanceRecord) {
		t.Fatalf("error = %v, want ErrInvalidAttendanceRecord", err)
	}
}

func TestNewAttendanceRecordRejectsOccurrenceOutsideWeek(t *testing.T) {
	input := validAttendanceInput()
	input.OccurredAt = input.WeekEnd.Add(time.Second)
	if _, err := NewAttendanceRecord(input); !errors.Is(err, ErrOccurredOutsideWeek) {
		t.Fatalf("error = %v, want ErrOccurredOutsideWeek", err)
	}
}

func TestNewAttendanceRecordIncludesEntireWeekEndDate(t *testing.T) {
	input := validAttendanceInput()
	input.WeekEnd = time.Date(2026, 6, 21, 0, 0, 0, 0, input.OccurredAt.Location())
	input.OccurredAt = time.Date(2026, 6, 21, 16, 30, 0, 0, input.OccurredAt.Location())
	if _, err := NewAttendanceRecord(input); err != nil {
		t.Fatalf("end-date occurrence rejected: %v", err)
	}
}

func TestNewAttendanceRecordRejectsAnotherInspectionType(t *testing.T) {
	input := validAttendanceInput()
	input.InspectionTypeCode = "CLASSROOM_HYGIENE"
	if _, err := NewAttendanceRecord(input); !errors.Is(err, ErrInspectionTypeMismatch) {
		t.Fatalf("error = %v, want ErrInspectionTypeMismatch", err)
	}
}

func TestNewAttendanceRecordUsesExcelDraftDefaultsAndStableDedupKey(t *testing.T) {
	input := validAttendanceInput()
	record, err := NewAttendanceRecord(input)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RecordStatusDraft || record.Source != RecordSourceExcel {
		t.Fatalf("status/source = %q/%q", record.Status, record.Source)
	}
	if record.DedupKey == "" {
		t.Fatal("dedup key must be generated")
	}
	again, err := NewAttendanceRecord(input)
	if err != nil {
		t.Fatal(err)
	}
	if again.DedupKey != record.DedupKey {
		t.Fatalf("dedup keys differ: %q != %q", again.DedupKey, record.DedupKey)
	}
	if restored := AttendanceDedupKey(record); restored != record.DedupKey {
		t.Fatalf("restored dedup key = %q, want %q", restored, record.DedupKey)
	}
}

func TestListAttendanceCompilesScopeIntoRepositoryQuery(t *testing.T) {
	repository := &recordingRepository{page: AttendancePage{Total: 0, Page: 2, PageSize: 25}}
	service := NewService(repository)
	page, err := service.ListAttendance(context.Background(), AttendanceListFilter{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "inspection-attendance",
		Keyword: "张晨", Status: RecordStatusDraft, Source: RecordSourceExcel, Sort: SortStudentNoAscending, Page: 2, PageSize: 25,
	}, AttendanceScope{DepartmentID: "department-study", ClassID: "class-1", Grade: "2023"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.query.SemesterID != "semester-1" || repository.query.WeekID != "week-3" || repository.query.InspectionTypeID != "inspection-attendance" {
		t.Fatalf("academic query = %+v", repository.query)
	}
	if repository.query.DepartmentID != "department-study" || repository.query.ClassID != "class-1" || repository.query.Grade != "2023" {
		t.Fatalf("scope query = %+v", repository.query)
	}
	if repository.query.Sort != SortStudentNoAscending || repository.query.Page != 2 || repository.query.PageSize != 25 {
		t.Fatalf("paging query = %+v", repository.query)
	}
	if repository.query.Status != RecordStatusDraft || repository.query.Source != RecordSourceExcel {
		t.Fatalf("business filters = %+v", repository.query)
	}
	if page.Items == nil {
		t.Fatal("empty attendance items must serialize as [] instead of null")
	}
}

func TestListAttendanceRejectsUnknownStatusOrSource(t *testing.T) {
	tests := []AttendanceListFilter{
		{SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "inspection-attendance", Status: "DELETED", Page: 1, PageSize: 20},
		{SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "inspection-attendance", Source: "UNKNOWN", Page: 1, PageSize: 20},
	}
	for _, filter := range tests {
		if _, err := NewService(&recordingRepository{}).ListAttendance(context.Background(), filter, AttendanceScope{}); !errors.Is(err, ErrInvalidAttendanceQuery) {
			t.Fatalf("filter=%+v error=%v, want ErrInvalidAttendanceQuery", filter, err)
		}
	}
}

func TestListAttendanceRejectsFilterOutsideCompiledScope(t *testing.T) {
	repository := &recordingRepository{}
	_, err := NewService(repository).ListAttendance(context.Background(), AttendanceListFilter{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "inspection-attendance",
		DepartmentID: "department-other", Page: 1, PageSize: 20,
	}, AttendanceScope{DepartmentID: "department-study"})
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("error = %v, want ErrScopeMismatch", err)
	}
	if repository.query.SemesterID != "" {
		t.Fatal("repository must not be called for a scope mismatch")
	}
}

func TestListAttendanceRejectsUnknownSort(t *testing.T) {
	_, err := NewService(&recordingRepository{}).ListAttendance(context.Background(), AttendanceListFilter{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "inspection-attendance",
		Sort: "studentName; DROP TABLE students", Page: 1, PageSize: 20,
	}, AttendanceScope{})
	if !errors.Is(err, ErrInvalidAttendanceQuery) {
		t.Fatalf("error = %v, want ErrInvalidAttendanceQuery", err)
	}
}
