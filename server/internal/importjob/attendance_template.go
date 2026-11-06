package importjob

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"

	"github.com/xuri/excelize/v2"
)

// BuildAttendanceTemplate builds the empty, version-locked workbook accepted by
// NewAttendanceXLSXParser. ZIP ordering and timestamps are fixed for stable hashes.
func BuildAttendanceTemplate() ([]byte, error) {
	return buildCourseTemplate("到课率导入", "ATTENDANCE_IMPORT",
		"填写说明：请保留前四行，从第5行填写；日期YYYY-MM-DD，节次填写如“第1-2节”，学时为实际旷课节次数量，地点为教学楼名称及教室号。",
		attendanceImportFields)
}

// BuildUnattendedTemplate keeps a distinct field contract: no time, period or hours.
func BuildUnattendedTemplate() ([]byte, error) {
	return buildCourseTemplate("无机课堂导入", "UNATTENDED_CLASSROOM_IMPORT",
		"填写说明：请保留前四行，从第5行填写；日期YYYY-MM-DD，地点为教学楼名称及教室号。每条记录固定计 1 次违纪，不填写节次与学时。",
		unattendedImportFields)
}

func buildCourseTemplate(sheet, code, instructions string, fields []TemplateField) ([]byte, error) {
	return buildImportTemplate(sheet, code, instructions, fields)
}

func buildImportTemplate(sheet, code, instructions string, fields []TemplateField) ([]byte, error) {
	lastColumn, err := excelize.ColumnNumberToName(len(fields))
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	rows := [][]any{{"template_code", code, "template_version", CurrentTemplateVersion}, {}, {}, {instructions}}
	for _, field := range fields {
		rows[1] = append(rows[1], field.Code)
		rows[2] = append(rows[2], field.Header)
	}
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return nil, err
		}
		if err := book.SetSheetRow(sheet, cell, &row); err != nil {
			return nil, err
		}
	}
	if err := book.SetRowVisible(sheet, 1, false); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(sheet, 2, false); err != nil {
		return nil, err
	}
	if err := book.MergeCell(sheet, "A4", lastColumn+"4"); err != nil {
		return nil, err
	}
	if err := book.SetColWidth(sheet, "A", lastColumn, 22); err != nil {
		return nil, err
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(sheet, "A5", lastColumn+"1004", textStyle); err != nil {
		return nil, err
	}
	if err := book.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 4, TopLeftCell: "A5", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		return nil, err
	}
	sort.Slice(archive.File, func(i, j int) bool { return archive.File[i].Name < archive.File[j].Name })
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	for _, entry := range archive.File {
		input, err := entry.Open()
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(input)
		closeErr := input.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		output, err := writer.CreateHeader(&zip.FileHeader{Name: entry.Name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := output.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
