package importjob

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var attendanceFields = []TemplateField{
	{Code: "student_no", Header: "学号"},
	{Code: "student_name", Header: "姓名"},
	{Code: "class_name", Header: "班级"},
	{Code: "course_name", Header: "课程"},
	{Code: "absence_date", Header: "旷课日期"},
	{Code: "absence_time", Header: "旷课时间"},
	{Code: "period", Header: "节次"},
	{Code: "absence_hours", Header: "旷课学时"},
	{Code: "classroom_code", Header: "上课地点"},
	{Code: "remark", Header: "备注"},
}

func attendanceParser(limits XLSXLimits) *XLSXParser {
	return NewXLSXParser(ParserConfig{
		ExpectedTemplateCode: "ATTENDANCE_IMPORT", ExpectedTemplateVersion: 1,
		ExpectedSheetName: "到课率导入", Fields: attendanceFields, DataStartRow: 5, Limits: limits,
	})
}

func defaultTestLimits() XLSXLimits {
	return XLSXLimits{
		MaxCompressedBytes: 2 << 20, MaxEntries: 128, MaxEntryUncompressedBytes: 4 << 20,
		MaxTotalUncompressedBytes: 8 << 20, MaxRows: 1000,
	}
}

func TestAttendanceParserFactoryLocksProductionRowLimit(t *testing.T) {
	parser := NewAttendanceXLSXParser(8 << 20)
	if parser.config.Limits.MaxRows != 1000 {
		t.Fatalf("production row limit = %d", parser.config.Limits.MaxRows)
	}
}

func attendanceRows(dataRows ...[]string) [][]string {
	codes := make([]string, len(attendanceFields))
	headers := make([]string, len(attendanceFields))
	for index, field := range attendanceFields {
		codes[index] = field.Code
		headers[index] = field.Header
	}
	rows := [][]string{
		{"template_code", "ATTENDANCE_IMPORT", "template_version", "1"},
		codes,
		headers,
		{"请勿修改第1-4行；旷课日期格式为 YYYY-MM-DD"},
	}
	return append(rows, dataRows...)
}

func validDataRow() []string {
	return []string{" S20260001 ", " 测试学生甲 ", " 示例班级A ", " 数据结构 ", "2026-06-17", "08:00", "第1-2节", "2", "A101", " 示例数据 "}
}

func TestXLSXParserParsesValidTemplateAndNormalizesBlankRows(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow(), []string{"", "  "})}}, nil)
	workbook, err := attendanceParser(defaultTestLimits()).Parse("attendance.xlsx", content)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.TemplateCode != "ATTENDANCE_IMPORT" || workbook.TemplateVersion != 1 || workbook.SheetName != "到课率导入" {
		t.Fatalf("metadata = %+v", workbook)
	}
	if len(workbook.Fields) != len(attendanceFields) || workbook.Fields[0] != attendanceFields[0] {
		t.Fatalf("fields = %+v", workbook.Fields)
	}
	if len(workbook.Rows) != 1 || workbook.Rows[0].RowNo != 5 {
		t.Fatalf("rows = %+v", workbook.Rows)
	}
	if workbook.Rows[0].Values["student_no"] != "S20260001" || workbook.Rows[0].Values["remark"] != "示例数据" {
		t.Fatalf("normalized row = %+v", workbook.Rows[0])
	}
}

func TestXLSXParserRejectsWrongExtension(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, nil)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsm", content, ErrorInvalidFileType)
}

func TestXLSXParserRejectsNonZIPAndEncryptedOfficeContainers(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{"plain text", []byte("not an xlsx")},
		{"encrypted OLE container", append([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, []byte("EncryptedPackage")...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", test.content, ErrorInvalidXLSXSignature)
		})
	}
}

func TestXLSXParserRejectsMacroParts(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, map[string]string{
		"xl/vbaProject.bin": "macro payload",
	})
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorInvalidFileType)
}

func TestXLSXParserRejectsUnsafeArchivePaths(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, map[string]string{
		"../outside.xml": "unsafe",
	})
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorXLSXArchiveLimitExceeded)
}

func TestXLSXParserRejectsSymbolicLinkArchiveEntries(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, nil)
	content = addSymlinkEntry(t, content)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorXLSXArchiveLimitExceeded)
}

func TestXLSXParserEnforcesArchiveLimitsBeforeWorkbookParsing(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, map[string]string{
		"customXml/large.xml": strings.Repeat("a", 2048),
	})
	limits := defaultTestLimits()
	limits.MaxEntryUncompressedBytes = 1024
	requireParserCode(t, attendanceParser(limits), "attendance.xlsx", content, ErrorXLSXArchiveLimitExceeded)

	limits = defaultTestLimits()
	limits.MaxEntries = 3
	requireParserCode(t, attendanceParser(limits), "attendance.xlsx", content, ErrorXLSXArchiveLimitExceeded)

	limits = defaultTestLimits()
	limits.MaxCompressedBytes = int64(len(content) - 1)
	requireParserCode(t, attendanceParser(limits), "attendance.xlsx", content, ErrorFileTooLarge)
}

func TestXLSXParserRejectsArchiveWhoseCombinedUncompressedSizeExceedsLimit(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, map[string]string{
		"customXml/part1.xml": strings.Repeat("a", 800),
		"customXml/part2.xml": strings.Repeat("b", 800),
	})
	limits := defaultTestLimits()
	limits.MaxEntryUncompressedBytes = 4096
	limits.MaxTotalUncompressedBytes = 1500
	requireParserCode(t, attendanceParser(limits), "attendance.xlsx", content, ErrorXLSXArchiveLimitExceeded)
}

func TestXLSXParserRejectsMultipleWorksheets(t *testing.T) {
	content := makeXLSX(t, []workbookSheet{
		{Name: "到课率导入", Rows: attendanceRows(validDataRow())},
		{Name: "隐藏数据", Rows: [][]string{{"unexpected"}}},
	}, nil)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorInvalidWorkbook)
}

func TestXLSXParserRejectsTemplateVersionAndHeaderMismatch(t *testing.T) {
	wrongVersion := attendanceRows(validDataRow())
	wrongVersion[0][3] = "2"
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: wrongVersion}}, nil)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorTemplateVersionMismatch)

	wrongHeader := attendanceRows(validDataRow())
	wrongHeader[2][0] = "学生编号"
	content = makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: wrongHeader}}, nil)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorInvalidHeader)
}

func TestXLSXParserAllows1000RowsAndRejects1001Rows(t *testing.T) {
	dataRows := make([][]string, 1001)
	for index := range dataRows {
		row := validDataRow()
		row[0] = fmt.Sprintf("S%08d", index+1)
		dataRows[index] = row
	}
	content := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(dataRows[:1000]...)}}, nil)
	workbook, err := attendanceParser(defaultTestLimits()).Parse("attendance.xlsx", content)
	if err != nil || len(workbook.Rows) != 1000 {
		t.Fatalf("1000 rows result = %d, error = %v", len(workbook.Rows), err)
	}
	content = makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(dataRows...)}}, nil)
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance.xlsx", content, ErrorRowLimitExceeded)
}

func TestAttendanceGoldenWorkbook(t *testing.T) {
	validContent, err := os.ReadFile(filepath.Join("testdata", "attendance-v1-valid.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := attendanceParser(defaultTestLimits()).Parse("attendance-v1-valid.xlsx", validContent)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.SheetName != "到课率导入" || len(workbook.Fields) != 10 || len(workbook.Rows) != 2 {
		t.Fatalf("golden workbook shape = sheet %q, fields %d, rows %d", workbook.SheetName, len(workbook.Fields), len(workbook.Rows))
	}
	if workbook.Rows[0].Values["student_no"] != "S20260001" || workbook.Rows[1].Values["student_name"] != "测试学生乙" {
		t.Fatalf("golden rows = %+v", workbook.Rows)
	}

	invalidContent, err := os.ReadFile(filepath.Join("testdata", "attendance-v1-invalid.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	requireParserCode(t, attendanceParser(defaultTestLimits()), "attendance-v1-invalid.xlsx", invalidContent, ErrorInvalidHeader)
}

func requireParserCode(t *testing.T, parser *XLSXParser, filename string, content []byte, code ErrorCode) {
	t.Helper()
	_, err := parser.Parse(filename, content)
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	var parseError *ParseError
	if !errors.As(err, &parseError) || parseError.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
}

type workbookSheet struct {
	Name string
	Rows [][]string
}

func makeXLSX(t *testing.T, sheets []workbookSheet, extraEntries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipEntry(t, writer, "[Content_Types].xml", contentTypesXML(len(sheets)))
	writeZipEntry(t, writer, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	writeZipEntry(t, writer, "xl/workbook.xml", workbookXML(sheets))
	writeZipEntry(t, writer, "xl/_rels/workbook.xml.rels", workbookRelationshipsXML(len(sheets)))
	for index, sheet := range sheets {
		writeZipEntry(t, writer, fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1), worksheetXML(sheet.Rows))
	}
	for name, content := range extraEntries {
		writeZipEntry(t, writer, name, content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeZipEntry(t *testing.T, writer *zip.Writer, name, content string) {
	t.Helper()
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}

func addSymlinkEntry(t *testing.T, content []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, source := range reader.File {
		target, err := writer.CreateHeader(&source.FileHeader)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := source.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(target, opened); err != nil {
			opened.Close()
			t.Fatal(err)
		}
		if err := opened.Close(); err != nil {
			t.Fatal(err)
		}
	}
	header := &zip.FileHeader{Name: "xl/unsafe-link", Method: zip.Store}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("../../outside")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func contentTypesXML(sheetCount int) string {
	var overrides strings.Builder
	for index := 1; index <= sheetCount; index++ {
		fmt.Fprintf(&overrides, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, index)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` + overrides.String() + `</Types>`
}

func workbookXML(sheets []workbookSheet) string {
	var values strings.Builder
	for index, sheet := range sheets {
		fmt.Fprintf(&values, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, html.EscapeString(sheet.Name), index+1, index+1)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` + values.String() + `</sheets></workbook>`
}

func workbookRelationshipsXML(sheetCount int) string {
	var values strings.Builder
	for index := 1; index <= sheetCount; index++ {
		fmt.Fprintf(&values, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, index, index)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + values.String() + `</Relationships>`
}

func worksheetXML(rows [][]string) string {
	var values strings.Builder
	for rowIndex, row := range rows {
		fmt.Fprintf(&values, `<row r="%d">`, rowIndex+1)
		for columnIndex, cell := range row {
			if cell == "" {
				continue
			}
			fmt.Fprintf(&values, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, columnName(columnIndex+1), rowIndex+1, html.EscapeString(cell))
		}
		values.WriteString(`</row>`)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` + values.String() + `</sheetData></worksheet>`
}

func columnName(column int) string {
	var result string
	for column > 0 {
		column--
		result = string(rune('A'+column%26)) + result
		column /= 26
	}
	return result
}

func TestXLSXParserRejectsFormulasHiddenSheetsAndExtraData(t *testing.T) {
	base := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, nil)
	for _, test := range []struct{ name, entry, old, replacement string }{
		{"cached formula", "xl/worksheets/sheet1.xml", `<row r="5">`, `<row r="5"><c r="K5"><f>1+1</f><v>2</v></c>`},
		{"shared formula", "xl/worksheets/sheet1.xml", `<row r="5">`, `<row r="5"><c r="K5"><f t="shared" si="0"/><v>2</v></c>`},
		{"hidden sheet", "xl/workbook.xml", `<sheet name=`, `<sheet state="hidden" name=`},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := rewriteWorkbookPart(t, base, test.entry, func(text string) string { return strings.Replace(text, test.old, test.replacement, 1) })
			requireParserCode(t, NewAttendanceXLSXParser(16<<20), "unsafe.xlsx", changed, ErrorInvalidWorkbook)
		})
	}
	extra := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(append(validDataRow(), "unexpected"))}}, nil)
	requireParserCode(t, NewAttendanceXLSXParser(16<<20), "extra.xlsx", extra, ErrorInvalidWorkbook)
}

func TestXLSXParserRejectsHiddenBusinessRowsAndColumns(t *testing.T) {
	base := makeXLSX(t, []workbookSheet{{Name: "到课率导入", Rows: attendanceRows(validDataRow())}}, nil)
	hiddenRow := rewriteWorkbookPart(t, base, "xl/worksheets/sheet1.xml", func(text string) string {
		return strings.Replace(text, `<row r="5">`, `<row r="5" hidden="1">`, 1)
	})
	requireParserCode(t, attendanceParser(defaultTestLimits()), "hidden-row.xlsx", hiddenRow, ErrorInvalidWorkbook)
	hiddenColumn := rewriteWorkbookPart(t, base, "xl/worksheets/sheet1.xml", func(text string) string {
		return strings.Replace(text, `<sheetData>`, `<cols><col min="1" max="1" hidden="1"/></cols><sheetData>`, 1)
	})
	requireParserCode(t, attendanceParser(defaultTestLimits()), "hidden-column.xlsx", hiddenColumn, ErrorInvalidWorkbook)
}

func rewriteWorkbookPart(t *testing.T, content []byte, name string, change func(string) string) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	for _, file := range archive.File {
		input, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(input)
		if err != nil {
			t.Fatal(err)
		}
		if err = input.Close(); err != nil {
			t.Fatal(err)
		}
		if file.Name == name {
			body = []byte(change(string(body)))
		}
		output, err := writer.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = output.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestAttendanceHeaderAcceptsNewAndPublishedLegacyWording(t *testing.T) {
	p := NewAttendanceXLSXParser(1 << 20)
	codes := make([]string, len(attendanceImportFields))
	headers := make([]string, len(attendanceImportFields))
	for i, field := range attendanceImportFields {
		codes[i] = field.Code
		headers[i] = field.Header
	}
	headers[7] = "旷课节次数"
	if !p.validHeaders(codes, headers) {
		t.Fatal("new period count header rejected")
	}
	headers[7] = "旷课学时"
	if !p.validHeaders(codes, headers) {
		t.Fatal("published legacy template rejected")
	}
}
