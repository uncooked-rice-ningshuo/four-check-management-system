package importjob

import (
	"bytes"
	"context"
	"errors"
	"four-check-management-system/server/internal/storage"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestUnattendedTemplateIsDistinctFromAttendance(t *testing.T) {
	content, err := BuildUnattendedTemplate()
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildUnattendedTemplate()
	if err != nil || !bytes.Equal(content, again) {
		t.Fatal("unstable template", err)
	}
	parser := NewUnattendedXLSXParser(16 << 20)
	parsed, err := parser.Parse("unattended.xlsx", content)
	if err != nil || parsed.TemplateCode != "UNATTENDED_CLASSROOM_IMPORT" || parsed.TemplateVersion != UnattendedTemplateVersion || len(parsed.Fields) != 8 {
		t.Fatalf("template %+v %v", parsed, err)
	}
	publisherParser, err := NewInspectionXLSXParser("UNATTENDED_CLASSROOM", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := publisherParser.Parse("unattended-classroom-v3.xlsx", content); err != nil || published.TemplateVersion != UnattendedTemplateVersion {
		t.Fatalf("publication parser rejected the current template: %+v %v", published, err)
	}
	if _, err = NewAttendanceXLSXParser(16<<20).Parse("unattended.xlsx", content); err == nil {
		t.Fatal("cross-type template accepted")
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"000001", "虚构学生", "示例班", "2026-10-01", "示例课", "第1-2节", "示例楼 101", ""}
	if err = book.SetSheetRow("无机课堂导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("filled.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].Values["student_no"] != "000001" {
		t.Fatal("lost text fields", err)
	}
	if err = book.SetCellFormula("无机课堂导入", "G5", "1+1"); err != nil {
		t.Fatal(err)
	}
	filled, err = book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	_, err = parser.Parse("formula.xlsx", filled.Bytes())
	var unsafe *ParseError
	if !errors.As(err, &unsafe) {
		t.Fatal("formula accepted", err)
	}
}

func TestCourseImportDispatchUsesAuthoritativeBatchType(t *testing.T) {
	unattended, err := BuildUnattendedTemplate()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		typeCode, templateCode string
		wantError              bool
	}{
		{"UNATTENDED_CLASSROOM", "UNATTENDED_CLASSROOM_IMPORT", true},
		{"ATTENDANCE_RATE", "ATTENDANCE_IMPORT", true},
		{"UNATTENDED_CLASSROOM", "ATTENDANCE_IMPORT", true},
		{"DORMITORY_HYGIENE", "UNATTENDED_CLASSROOM_IMPORT", true},
	} {
		repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
		repository.batch.TypeCode, repository.batch.TemplateCode = test.typeCode, test.templateCode
		if test.typeCode == "UNATTENDED_CLASSROOM" {
			repository.batch.TemplateVersion = UnattendedTemplateVersion
		}
		validator := &fakeAttendanceValidator{result: AttendanceValidationResult{Rows: []AttendanceValidationRow{}}}
		service := NewCourseImportService(repository, &fakeVerifiedFileReader{file: storage.File{OriginalName: "source.xlsx"}, content: unattended}, validator, 16<<20)
		_, err := service.ParseBatch(context.Background(), ParseBatchInput{BatchID: repository.batch.ID, ExpectedVersion: repository.batch.Version})
		if (err != nil) != test.wantError {
			t.Fatalf("type=%s template=%s error=%v", test.typeCode, test.templateCode, err)
		}
		if test.typeCode == "UNATTENDED_CLASSROOM" && test.templateCode == "UNATTENDED_CLASSROOM_IMPORT" && repository.batch.FailureMessage != "表格没有可导入的业务数据行" {
			t.Fatalf("valid template dispatch failure = %+v", repository.batch)
		}
		if test.wantError && validator.calls != 0 {
			t.Fatal("wrong template reached business validation")
		}
	}
}
