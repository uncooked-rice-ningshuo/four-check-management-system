package importjob

import (
	"bytes"
	"four-check-management-system/server/internal/inspection"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestClassroomHygieneTemplateContract(t *testing.T) {
	data, err := BuildInspectionTemplate(inspection.InspectionTypeClassroomHygiene)
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	fields := []string{"classroom_name", "area_name", "inspection_date", "inspection_time", "inspection_round", "result", "responsible_class_names", "responsible_student_nos", "reason_name", "description", "remark"}
	values := []string{"教学楼 101", "前排", "2026-10-01", "", "首轮", "不合格", "示例班", "000001", "地面", "对象检查", "备注"}
	for i, field := range fields {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		got, err := book.GetCellValue("教室卫生导入", cell)
		if err != nil || got != field {
			t.Fatalf("field %d: %s %v", i, got, err)
		}
		cell, _ = excelize.CoordinatesToCellName(i+1, 5)
		if err := book.SetCellStr("教室卫生导入", cell, values[i]); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parser, err := NewInspectionXLSXParser(inspection.InspectionTypeClassroomHygiene, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parser.Parse("classroom.xlsx", filled.Bytes())
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].Values["responsible_student_nos"] != "000001" || rows.Rows[0].Values["area_name"] != "前排" {
		t.Fatal(rows, err)
	}
	dorm, err := NewInspectionXLSXParser(inspection.InspectionTypeDormitoryHygiene, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dorm.Parse("classroom.xlsx", filled.Bytes()); err == nil {
		t.Fatal("cross-type template accepted")
	}
	again, err := BuildInspectionTemplate(inspection.InspectionTypeClassroomHygiene)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("template bytes not stable", err)
	}
}
