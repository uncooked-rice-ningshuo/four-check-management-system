package importjob

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"reflect"
	"testing"
)

func TestMorningTemplateUsesStudentDayFields(t *testing.T) {
	content, err := BuildMorningTemplate()
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildMorningTemplate()
	if err != nil || !bytes.Equal(content, again) {
		t.Fatal("unstable morning template", err)
	}
	parser := NewMorningXLSXParser(16 << 20)
	parsed, err := parser.Parse("morning.xlsx", content)
	expected := []TemplateField{{Code: "student_no", Header: "学号"}, {Code: "student_name", Header: "姓名"}, {Code: "class_name", Header: "班级"}, {Code: "inspection_date", Header: "检查日期"}, {Code: "remark", Header: "备注"}}
	if err != nil || parsed.TemplateCode != "MORNING_ATTENDANCE_IMPORT" || parsed.TemplateVersion != 1 || !reflect.DeepEqual(parsed.Fields, expected) || len(parsed.Rows) != 0 {
		t.Fatalf("invalid template %+v %v", parsed, err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"000001", "虚构学生", "示例班", "2026-10-01", "单次缺勤"}
	if err = book.SetSheetRow("早锻炼早签导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("morning.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].Values["student_no"] != "000001" || parsed.Rows[0].Values["inspection_date"] != "2026-10-01" {
		t.Fatal("date or leading zero lost", parsed, err)
	}
	if _, err = NewAttendanceXLSXParser(16<<20).Parse("morning.xlsx", filled.Bytes()); err == nil {
		t.Fatal("morning template accepted as attendance")
	}
}

func TestMorningTemplateInheritsWorkbookSafety(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*excelize.File) error
	}{
		{"wrong header", func(f *excelize.File) error { return f.SetCellStr("早锻炼早签导入", "D3", "累计缺勤次数") }},
		{"formula", func(f *excelize.File) error { return f.SetCellFormula("早锻炼早签导入", "E5", "1+1") }},
		{"extra data", func(f *excelize.File) error {
			return f.SetCellStr("早锻炼早签导入", "F5", "不接受累计值或虚构时刻")
		}},
		{"extra sheet", func(f *excelize.File) error { _, err := f.NewSheet("其他业务"); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := BuildMorningTemplate()
			if err != nil {
				t.Fatal(err)
			}
			book, err := excelize.OpenReader(bytes.NewReader(content))
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()
			if err = test.change(book); err != nil {
				t.Fatal(err)
			}
			filled, err := book.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = NewMorningXLSXParser(16<<20).Parse("morning.xlsx", filled.Bytes()); err == nil {
				t.Fatal("unsafe workbook accepted")
			}
		})
	}
}
