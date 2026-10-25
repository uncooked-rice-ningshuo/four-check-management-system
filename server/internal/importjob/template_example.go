package importjob

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExampleTemplateCopy adds a visible, non-imported example to a verified published
// workbook. The published object and its version stay immutable; parsers skip row 4.
func ExampleTemplateCopy(content []byte) ([]byte, error) {
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) != 1 {
		return nil, fmt.Errorf("expected one import worksheet")
	}
	sheet := sheets[0]
	rows, err := book.GetRows(sheet)
	if err != nil || len(rows) < 4 || len(rows[1]) == 0 {
		return nil, fmt.Errorf("missing import template headers: %w", err)
	}
	code := ""
	if len(rows[0]) > 1 {
		code = rows[0][1]
	}
	if !strings.HasSuffix(code, "_IMPORT") {
		return nil, fmt.Errorf("invalid import template code")
	}
	instructions := ""
	if len(rows[3]) > 0 {
		instructions = rows[3][0]
	}
	lastColumn, err := excelize.ColumnNumberToName(len(rows[1]))
	if err != nil {
		return nil, err
	}
	if err := book.UnmergeCell(sheet, "A4", lastColumn+"4"); err != nil {
		return nil, err
	}
	for index, field := range rows[1] {
		cell, err := excelize.CoordinatesToCellName(index+1, 4)
		if err != nil {
			return nil, err
		}
		if err := book.SetCellValue(sheet, cell, exampleValue(code, field)); err != nil {
			return nil, err
		}
	}
	if err := book.AddComment(sheet, excelize.Comment{Cell: "A4", Author: "系统", Text: "第4行是填写示例，不参与导入。" + instructions}); err != nil {
		return nil, err
	}
	if err := book.SetRowHeight(sheet, 4, 30); err != nil {
		return nil, err
	}
	style, err := book.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Color: "666666", Italic: true}})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(sheet, "A4", lastColumn+"4", style); err != nil {
		return nil, err
	}
	result, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func exampleValue(code, field string) string {
	switch field {
	case "student_no", "responsible_student_nos":
		return "000001"
	case "student_name":
		return "示例同学"
	case "class_name", "responsible_class_names":
		return "示例班"
	case "course_name":
		return "示例课程"
	case "classroom_code", "classroom_name":
		return "示例教学楼 101"
	case "dormitory_name", "responsible_dormitory_names":
		return "示例区 示例楼 101"
	case "area_name":
		return "示例区域"
	case "absence_date", "inspection_date":
		return "2026-10-01"
	case "absence_time", "inspection_time":
		if strings.Contains(code, "HYGIENE") {
			return "08:00:00"
		}
		return "08:00"
	case "period":
		return "1-2"
	case "absence_hours":
		return "2"
	case "result":
		return "合格"
	case "inspection_round":
		return ""
	case "reason_name":
		return ""
	case "description":
		return "示例情况说明"
	case "remark":
		return "示例备注"
	default:
		return "示例"
	}
}
