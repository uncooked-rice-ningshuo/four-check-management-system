package importjob

import (
	"bytes"
	"four-check-management-system/server/internal/inspection"
	"github.com/xuri/excelize/v2"
	"reflect"
	"testing"
)

func TestDormitoryHygieneTemplateObjectFields(t *testing.T) {
	content, err := BuildInspectionTemplate(inspection.InspectionTypeDormitoryHygiene)
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildInspectionTemplate(inspection.InspectionTypeDormitoryHygiene)
	if err != nil || !bytes.Equal(content, again) {
		t.Fatal("unstable template", err)
	}
	parser, err := NewInspectionXLSXParser(inspection.InspectionTypeDormitoryHygiene, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("hygiene.xlsx", content)
	fields := []TemplateField{{Code: "dormitory_name", Header: "宿舍完整名称"}, {Code: "inspection_date", Header: "检查日期"}, {Code: "inspection_time", Header: "检查时间"}, {Code: "inspection_round", Header: "检查轮次"}, {Code: "result", Header: "检查结果"}, {Code: "responsible_class_names", Header: "责任班级"}, {Code: "responsible_student_nos", Header: "责任学生学号"}, {Code: "reason_name", Header: "原因名称"}, {Code: "description", Header: "情况说明"}, {Code: "remark", Header: "备注"}}
	if err != nil || parsed.TemplateCode != "DORMITORY_HYGIENE_IMPORT" || parsed.TemplateVersion != 1 || !reflect.DeepEqual(parsed.Fields, fields) || len(parsed.Rows) != 0 {
		t.Fatal("invalid template", parsed, err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	row := []any{"示例区 示例楼 101", "2026-10-01", "", "第1轮", "优秀", "示例班甲;示例班乙", "000001;000002", "", "整体检查", "示例"}
	if err = book.SetSheetRow("宿舍卫生导入", "A5", &row); err != nil {
		t.Fatal(err)
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parser.Parse("hygiene.xlsx", filled.Bytes())
	if err != nil || len(parsed.Rows) != 1 || parsed.Rows[0].Values["responsible_student_nos"] != "000001;000002" || parsed.Rows[0].Values["inspection_time"] != "" {
		t.Fatal("responsibilities or granularity lost", parsed, err)
	}
	if _, err := NewLateReturnXLSXParser(16<<20).Parse("hygiene.xlsx", filled.Bytes()); err == nil {
		t.Fatal("hygiene accepted as late return")
	}
}
