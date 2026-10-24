package academicimport

import (
	"four-check-management-system/server/internal/importjob"
	"github.com/xuri/excelize/v2"
)

const (
	TemplateCode    = "ACADEMIC_WEEKS"
	TemplateVersion = 2
	SheetName       = "周次导入"
	maxRows         = 60
)

type FieldSpec struct {
	Code     string `json:"code"`
	Header   string `json:"header"`
	Required bool   `json:"required"`
	Format   string `json:"format"`
}

type Spec struct {
	TemplateCode    string      `json:"templateCode"`
	TemplateVersion int         `json:"templateVersion"`
	SheetName       string      `json:"sheetName"`
	MaxRows         int         `json:"maxRows"`
	Fields          []FieldSpec `json:"fields"`
}

var fields = []FieldSpec{
	{Code: "week_no", Header: "周次", Required: true, Format: "整数；必填，ASCII 十进制整数，范围 1–60；目标学期内不能重复。"},
	{Code: "start_date", Header: "开始日期", Required: true, Format: "日期文本；必填，严格 YYYY-MM-DD；必须位于所选学期范围内。"},
	{Code: "end_date", Header: "结束日期", Required: true, Format: "日期文本；必填，严格 YYYY-MM-DD；不得早于开始日期，且必须位于所选学期范围内。"},
	{Code: "note", Header: "备注", Format: "文本；选填，最多 255 个 Unicode 字符；空白按空备注处理。"},
	{Code: "attendance_penalty_multiplier", Header: "到课率违纪计入倍数", Format: "整数；选填，只能填写 1 或 2；留空按普通周 1 倍处理。"},
}

var exampleRow = []any{"1", "2026-09-01", "2026-09-07", "示例备注", "1"}

func GetSpec() Spec {
	result := Spec{TemplateCode: TemplateCode, TemplateVersion: TemplateVersion, SheetName: SheetName, MaxRows: maxRows}
	result.Fields = append([]FieldSpec(nil), fields...)
	return result
}

func NewParser(maxCompressedBytes int64) (*importjob.XLSXParser, error) {
	if maxCompressedBytes <= 0 {
		return nil, ErrInvalidBatch
	}
	parserFields := make([]importjob.TemplateField, len(fields))
	for index, field := range fields {
		parserFields[index] = importjob.TemplateField{Code: field.Code, Header: field.Header}
	}
	return importjob.NewXLSXParser(importjob.ParserConfig{
		ExpectedTemplateCode: TemplateCode, ExpectedTemplateVersion: TemplateVersion,
		ExpectedSheetName: SheetName, Fields: parserFields, DataStartRow: 6,
		Limits: importjob.XLSXLimits{MaxCompressedBytes: maxCompressedBytes, MaxEntries: 512,
			MaxEntryUncompressedBytes: 16 << 20, MaxTotalUncompressedBytes: 64 << 20, MaxRows: maxRows},
	}), nil
}

func TemplateXLSX() ([]byte, error) {
	if len(exampleRow) != len(fields) {
		return nil, ErrInvalidBatch
	}
	lastColumn, err := excelize.ColumnNumberToName(len(fields))
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", SheetName); err != nil {
		return nil, err
	}
	rows := [][]any{{"template_code", TemplateCode, "template_version", TemplateVersion}, {}, {}, {}}
	for _, field := range fields {
		rows[1] = append(rows[1], field.Code)
		rows[2] = append(rows[2], field.Header)
	}
	for _, field := range fields {
		rows[3] = append(rows[3], field.Format)
	}
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			return nil, err
		}
		if err := book.SetSheetRow(SheetName, cell, &row); err != nil {
			return nil, err
		}
	}
	if err := book.SetSheetRow(SheetName, "A5", &exampleRow); err != nil {
		return nil, err
	}
	if err := book.AddComment(SheetName, excelize.Comment{
		Cell:      "A5",
		Author:    "学期周次导入",
		Paragraph: []excelize.RichTextRun{{Text: "第 4 行逐列说明字段格式，第 5 行为示例且导入时自动忽略。请从第 6 行开始填写；周次号和日期须适用于抽屉中选择的目标学期，周次号也不能与该学期已有周次重复。"}},
		Height:    70,
		Width:     320,
	}); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(SheetName, 1, false); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(SheetName, 2, false); err != nil {
		return nil, err
	}
	widths := []float64{12, 16, 16, 30, 24}
	for index, width := range widths {
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		if err := book.SetColWidth(SheetName, column, column, width); err != nil {
			return nil, err
		}
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(SheetName, "A6", lastColumn+"65", textStyle); err != nil {
		return nil, err
	}
	formatStyle, err := book.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "404040", Size: 9},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"F3F6FA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(SheetName, "A4", lastColumn+"4", formatStyle); err != nil {
		return nil, err
	}
	exampleStyle, err := book.NewStyle(&excelize.Style{
		NumFmt: 49,
		Font:   &excelize.Font{Italic: true, Color: "666666"},
		Fill:   excelize.Fill{Type: "pattern", Color: []string{"EAF3F8"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(SheetName, "A5", lastColumn+"5", exampleStyle); err != nil {
		return nil, err
	}
	if err := book.SetRowHeight(SheetName, 4, 110); err != nil {
		return nil, err
	}
	if err := book.SetRowHeight(SheetName, 5, 24); err != nil {
		return nil, err
	}
	if err := book.SetPanes(SheetName, &excelize.Panes{Freeze: true, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
