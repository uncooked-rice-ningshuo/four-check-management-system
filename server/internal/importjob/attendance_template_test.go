package importjob

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestAttendanceTemplateMatchesParserAndPreservesText(t *testing.T) {
	content, err := BuildAttendanceTemplate()
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildAttendanceTemplate()
	if err != nil || !bytes.Equal(content, again) {
		t.Fatal("template v1 bytes are not reproducible", err)
	}
	parser := NewAttendanceXLSXParser(16 << 20)
	parsed, err := parser.Parse("attendance-v1.xlsx", content)
	if err != nil || parsed.TemplateCode != "ATTENDANCE_IMPORT" || parsed.TemplateVersion != 1 || len(parsed.Rows) != 0 || len(parsed.Fields) != 10 {
		t.Fatalf("invalid empty template: %+v %v", parsed, err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"000001", "虚构学生", "示例班", "示例课", "2026-09-01", "08:00", "1-2", "2", "示例楼 101", ""}
	if err := book.SetSheetRow("到课率导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("filled.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].RowNo != 5 || parsed.Rows[0].Values["student_no"] != "000001" {
		t.Fatalf("template cannot be filled: %+v %v", parsed, err)
	}
}
