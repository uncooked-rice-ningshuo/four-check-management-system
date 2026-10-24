package academicimport

import (
	"bytes"
	"context"
	"testing"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"github.com/xuri/excelize/v2"
)

type testCalendar struct{ calendar academic.Calendar }

func (c testCalendar) List(context.Context) (academic.Calendar, error) { return c.calendar, nil }

type testRepository struct{ batch Batch }

func (r *testRepository) CreateAcademicWeekImportBatch(_ context.Context, batch Batch, _ audit.Metadata) error {
	r.batch = batch
	return nil
}
func (r *testRepository) FindAcademicWeekImportBatch(context.Context, string, string) (Batch, error) {
	return r.batch, nil
}
func (r *testRepository) ConfirmAcademicWeekImportBatch(context.Context, ConfirmInput) (ConfirmResult, error) {
	return ConfirmResult{}, nil
}

func TestTemplateRoundTripsThroughLockedParser(t *testing.T) {
	content := workbookWithRows(t, [][]any{{"1", "2026-09-01", "2026-09-07", "开学周", ""}})
	parser, err := NewParser(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := parser.Parse("weeks.xlsx", content)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.TemplateCode != TemplateCode || workbook.TemplateVersion != 2 || len(workbook.Rows) != 1 || workbook.Rows[0].RowNo != 6 || workbook.Rows[0].Values["week_no"] != "1" {
		t.Fatalf("parsed workbook = %+v", workbook)
	}
	if got := len(GetSpec().Fields); got != 5 {
		t.Fatalf("field count = %d", got)
	}
}

func TestTemplateShowsFieldFormatsAndExampleBeforeDataRows(t *testing.T) {
	content, err := TemplateXLSX()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()

	rows, err := book.GetRows(SheetName)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 5 {
		t.Fatalf("template rows = %d, want format and example rows", len(rows))
	}
	wantFormats := make([]string, len(fields))
	for index, field := range fields {
		wantFormats[index] = field.Format
	}
	if len(rows[3]) != len(wantFormats) {
		t.Fatalf("row 4 format columns = %d, want %d", len(rows[3]), len(wantFormats))
	}
	for index := range wantFormats {
		if rows[3][index] != wantFormats[index] {
			t.Errorf("row 4 column %d format = %q, want %q", index+1, rows[3][index], wantFormats[index])
		}
	}
	wantExample := []string{"1", "2026-09-01", "2026-09-07", "示例备注", "1"}
	if len(rows[4]) != len(wantExample) {
		t.Fatalf("row 5 example columns = %d, want %d", len(rows[4]), len(wantExample))
	}
	for index := range wantExample {
		if rows[4][index] != wantExample[index] {
			t.Errorf("row 5 column %d example = %q, want %q", index+1, rows[4][index], wantExample[index])
		}
	}

	parser, err := NewParser(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("weeks.xlsx", content)
	if err != nil {
		t.Fatalf("template example row should be ignored: %v", err)
	}
	if len(parsed.Rows) != 0 {
		t.Fatalf("template example was parsed as business data: %+v", parsed.Rows)
	}

	oldTemplate, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer oldTemplate.Close()
	if err := oldTemplate.SetCellValue(SheetName, "D1", 1); err != nil {
		t.Fatal(err)
	}
	oldContent, err := oldTemplate.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse("old-weeks.xlsx", oldContent.Bytes()); err == nil {
		t.Fatal("parser accepted version 1 workbook with the old data start row")
	}
}

func TestParserRejectsRowsOverLimit(t *testing.T) {
	rows := make([][]any, maxRows+1)
	for index := range rows {
		rows[index] = []any{"1", "2026-09-01", "2026-09-07", "", ""}
	}
	parser, err := NewParser(4 << 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse("weeks.xlsx", workbookWithRows(t, rows)); err == nil {
		t.Fatal("parser accepted more than 60 rows")
	}
}

func TestPreviewAppliesRowRulesAndShowsTargetContext(t *testing.T) {
	semester := academic.Semester{ID: "semester-1", AcademicYearID: "year-1", Name: "2026-2027 学年上学期", StartDate: date("2026-09-01"), EndDate: date("2027-01-31"), Enabled: false}
	repository := &testRepository{}
	service := NewService(repository, testCalendar{academic.Calendar{
		AcademicYears: []academic.AcademicYear{{ID: "year-1", Name: "2026-2027 学年"}},
		Semesters:     []academic.Semester{semester},
		Weeks:         []academic.Week{{ID: "existing-disabled", SemesterID: semester.ID, WeekNo: 5, StartDate: date("2026-10-01"), EndDate: date("2026-10-07"), Enabled: false}},
	}}, 1<<20)
	service.now = func() time.Time { return time.Date(2026, 9, 2, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)) }
	rows := [][]any{
		{"1", "2026-09-01", "2026-09-07", " 开学 ", ""},
		{"5", "2026-10-01", "2026-10-07", "", "3"},
		{"1", "2026-09-08", "2026-09-14", "重复", "2"},
	}
	batch, err := service.Preview(context.Background(), semester.ID, "file-1", "weeks.xlsx", "admin-1", workbookWithRows(t, rows), audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != StatusPreviewHasErrors || batch.ValidRows != 0 || batch.InvalidRows != 3 || batch.SemesterName != semester.Name || batch.AcademicYearName == "" || !batch.AffectsToday {
		t.Fatalf("batch metadata = %+v", batch)
	}
	if batch.Rows[0].Week == nil || batch.Rows[0].Week.Note != "开学" || *batch.Rows[0].Week.AttendancePenaltyMultiplier != 1 {
		t.Fatalf("week defaults/normalization = %+v", batch.Rows[0])
	}
	if !hasFieldError(batch.Rows[0], "week_no") || !hasFieldError(batch.Rows[1], "week_no") || !hasFieldError(batch.Rows[1], "attendance_penalty_multiplier") || !hasFieldError(batch.Rows[2], "week_no") {
		t.Fatalf("row errors were not attributed: %+v", batch.Rows)
	}
	if repository.batch.ID != batch.ID {
		t.Fatalf("persisted batch id = %q, preview id = %q", repository.batch.ID, batch.ID)
	}
}

func TestValidateRowsRejectsDatesOutsideSemesterAndInclusiveOverlap(t *testing.T) {
	semester := academic.Semester{ID: "semester", StartDate: date("2026-09-01"), EndDate: date("2027-01-31")}
	parsed := []importjob.ParsedRow{
		{RowNo: 6, Values: map[string]string{"week_no": "1", "start_date": "2026-08-31", "end_date": "2026-09-06"}},
		{RowNo: 7, Values: map[string]string{"week_no": "2", "start_date": "2026-09-06", "end_date": "2026-09-12"}},
	}
	rows, _ := validateRows(parsed, semester, nil, date("2026-09-01"))
	if !hasFieldError(rows[0], "start_date") || !hasFieldError(rows[0], "start_date") || !hasFieldError(rows[1], "start_date") {
		t.Fatalf("expected semester and inclusive overlap errors: %+v", rows)
	}
}

func hasFieldError(row PreviewRow, field string) bool {
	for _, rowError := range row.Errors {
		if rowError.Field == field {
			return true
		}
	}
	return false
}

func workbookWithRows(t *testing.T, rows [][]any) []byte {
	t.Helper()
	content, err := TemplateXLSX()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+6)
		if err != nil {
			t.Fatal(err)
		}
		if err := book.SetSheetRow(SheetName, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func date(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}
