package importjob

import (
	"bytes"
	"four-check-management-system/server/internal/inspection"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestPublicDormitoryHygieneTemplateContract(t *testing.T) {
	data, err := BuildPublicDormitoryHygieneTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	fields := []string{"area_name", "inspection_date", "inspection_time", "inspection_round", "result", "responsible_class_names", "responsible_student_nos", "responsible_dormitory_names", "reason_name", "description", "remark"}
	values := []string{"示例区 示例楼 / 一楼 / 大厅", "2026-10-01", "", "首轮", "不合格", "示例班", "000001", "示例区 示例楼 101;示例区 示例楼 102", "地面", "对象检查", "备注"}
	for i, field := range fields {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		got, err := book.GetCellValue("公共宿舍卫生导入", cell)
		if err != nil || got != field {
			t.Fatalf("field %d: %s %v", i, got, err)
		}
		cell, _ = excelize.CoordinatesToCellName(i+1, 5)
		if err := book.SetCellStr("公共宿舍卫生导入", cell, values[i]); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parser := NewPublicDormitoryHygieneXLSXParser(16 << 20)
	rows, err := parser.Parse("public-area.xlsx", filled.Bytes())
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].Values["responsible_student_nos"] != "000001" || rows.Rows[0].Values["responsible_dormitory_names"] != "示例区 示例楼 101;示例区 示例楼 102" {
		t.Fatal(rows, err)
	}
	dorm, err := NewInspectionXLSXParser(inspection.InspectionTypeDormitoryHygiene, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dorm.Parse("public-area.xlsx", filled.Bytes()); err == nil {
		t.Fatal("cross-type template accepted")
	}
	again, err := BuildPublicDormitoryHygieneTemplate()
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("template bytes not stable", err)
	}
}

func TestPublicDormitoryHygieneImportRemainsUnregisteredPendingScopeDecision(t *testing.T) {
	if _, err := InspectionTemplate(inspection.InspectionTypePublicDormitoryHygiene); err == nil {
		t.Fatal("unapproved import scope enabled")
	}
}
