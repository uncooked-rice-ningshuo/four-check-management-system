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
		t.Fatal("attendance template bytes are not reproducible", err)
	}
	parser := NewAttendanceXLSXParser(16 << 20)
	parsed, err := parser.Parse("attendance-v2.xlsx", content)
	if err != nil || parsed.TemplateCode != "ATTENDANCE_IMPORT" || parsed.TemplateVersion != 2 || len(parsed.Rows) != 0 || len(parsed.Fields) != 9 {
		t.Fatalf("invalid empty template: %+v %v", parsed, err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"000001", "虚构学生", "示例班", "2026-09-01", "示例课", "1-2", "2", "示例楼 101", ""}
	if err := book.SetSheetRow("到课率导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("filled.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].RowNo != 5 || parsed.Rows[0].Values["student_no"] != "000001" || parsed.Rows[0].Values["absence_date"] != "2026-09-01" || parsed.Rows[0].Values["course_name"] != "示例课" {
		t.Fatalf("template cannot be filled: %+v %v", parsed, err)
	}
}
