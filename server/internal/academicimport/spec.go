package academicimport

import (
	"four-check-management-system/server/internal/importjob"
	"github.com/xuri/excelize/v2"
)

const (
	TemplateCode    = "ACADEMIC_WEEKS"
	TemplateVersion = 1
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
	{Code: "week_no", Header: "周次", Required: true, Format: "必填，ASCII 十进制整数，范围 1–60；目标学期内不能重复。"},
	{Code: "start_date", Header: "开始日期", Required: true, Format: "必填，严格 YYYY-MM-DD；必须位于所属学期内。"},
	{Code: "end_date", Header: "结束日期", Required: true, Format: "必填，严格 YYYY-MM-DD；不得早于开始日期，必须位于所属学期内。"},
	{Code: "note", Header: "备注", Format: "选填，最多 255 个 Unicode 字符。"},
	{Code: "attendance_penalty_multiplier", Header: "到课率违纪计入倍数", Format: "选填，只能填写 1 或 2；留空按普通周 1 倍处理。"},
}

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
		ExpectedSheetName: SheetName, Fields: parserFields, DataStartRow: 5,
		Limits: importjob.XLSXLimits{MaxCompressedBytes: maxCompressedBytes, MaxEntries: 512,
			MaxEntryUncompressedBytes: 16 << 20, MaxTotalUncompressedBytes: 64 << 20, MaxRows: maxRows},
	}), nil
}

func TemplateXLSX() ([]byte, error) {
	lastColumn, err := excelize.ColumnNumberToName(len(fields))
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", SheetName); err != nil {
		return nil, err
	}
	rows := [][]any{{"template_code", TemplateCode, "template_version", TemplateVersion}, {}, {}, {templateInstructions()}}
	for _, field := range fields {
		rows[1] = append(rows[1], field.Code)
		rows[2] = append(rows[2], field.Header)
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
	if err := book.SetRowVisible(SheetName, 1, false); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(SheetName, 2, false); err != nil {
		return nil, err
	}
	if err := book.MergeCell(SheetName, "A4", lastColumn+"4"); err != nil {
		return nil, err
	}
	if err := book.SetColWidth(SheetName, "A", lastColumn, 24); err != nil {
		return nil, err
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(SheetName, "A5", lastColumn+"64", textStyle); err != nil {
		return nil, err
	}
	if err := book.SetPanes(SheetName, &excelize.Panes{Freeze: true, YSplit: 4, TopLeftCell: "A5", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func templateInstructions() string {
	return "只填写第5行起的数据；保留前四行、字段顺序和工作表名称；最多60行；日期格式YYYY-MM-DD；不支持修改已有周次；全部行有效后才能整批确认。"
}
