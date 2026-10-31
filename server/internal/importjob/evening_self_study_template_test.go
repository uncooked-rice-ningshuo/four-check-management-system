package importjob

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"reflect"
	"testing"
)

func TestEveningSelfStudyTemplateContainsOnlyAbsenceEventFields(t *testing.T) {
	content, err := BuildEveningSelfStudyTemplate()
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildEveningSelfStudyTemplate()
	if err != nil || !bytes.Equal(content, again) {
		t.Fatalf("unstable evening self-study template: %v", err)
	}
	parser, err := NewInspectionXLSXParser("EVENING_SELF_STUDY", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("evening-self-study.xlsx", content)
	expected := []TemplateField{
		{Code: "student_no", Header: "学号"}, {Code: "student_name", Header: "姓名"},
		{Code: "class_name", Header: "班级"}, {Code: "classroom_code", Header: "教室"},
		{Code: "absence_date", Header: "缺勤日期"}, {Code: "absence_time", Header: "检查时间"},
		{Code: "remark", Header: "备注"},
	}
	if err != nil || parsed.TemplateCode != "EVENING_SELF_STUDY_IMPORT" || parsed.TemplateVersion != 1 || !reflect.DeepEqual(parsed.Fields, expected) || len(parsed.Rows) != 0 {
		t.Fatalf("invalid template: %#v, %v", parsed, err)
	}
	for _, forbidden := range []string{"course_name", "course_id", "period", "absence_hours"} {
		for _, field := range parsed.Fields {
			if field.Code == forbidden {
				t.Fatalf("template exposes unsupported field %q", forbidden)
			}
		}
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"S20260001", "测试学生甲", "示例班级A", "A101", "2026-06-17", "20:15", "晚自习缺勤"}
	if err := book.SetSheetRow("晚自习导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("evening-self-study.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].Values["student_no"] != "S20260001" || parsed.Rows[0].Values["absence_date"] != "2026-06-17" {
		t.Fatalf("filled template did not round-trip: %#v, %v", parsed, err)
	}
}
