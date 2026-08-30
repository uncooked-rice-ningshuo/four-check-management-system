package importjob

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type ErrorCode string

const (
	ErrorInvalidFileType          ErrorCode = "INVALID_FILE_TYPE"
	ErrorInvalidXLSXSignature     ErrorCode = "INVALID_XLSX_SIGNATURE"
	ErrorFileTooLarge             ErrorCode = "FILE_TOO_LARGE"
	ErrorXLSXArchiveLimitExceeded ErrorCode = "XLSX_ARCHIVE_LIMIT_EXCEEDED"
	ErrorTemplateVersionMismatch  ErrorCode = "TEMPLATE_VERSION_MISMATCH"
	ErrorInvalidHeader            ErrorCode = "INVALID_HEADER"
	ErrorRowLimitExceeded         ErrorCode = "ROW_LIMIT_EXCEEDED"
	ErrorInvalidWorkbook          ErrorCode = "INVALID_WORKBOOK"
)

// ParseError exposes only a stable code and safe message; parser internals never reach API responses.
type ParseError struct {
	Code    ErrorCode
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type TemplateField struct {
	Code   string `json:"code"`
	Header string `json:"header"`
}

type XLSXLimits struct {
	MaxCompressedBytes        int64
	MaxEntries                int
	MaxEntryUncompressedBytes uint64
	MaxTotalUncompressedBytes uint64
	MaxRows                   int
}

type ParserConfig struct {
	ExpectedTemplateCode    string
	ExpectedTemplateVersion int
	ExpectedSheetName       string
	Fields                  []TemplateField
	DataStartRow            int
	Limits                  XLSXLimits
}

type ParsedRow struct {
	RowNo  int               `json:"rowNo"`
	Values map[string]string `json:"values"`
}

type ParsedWorkbook struct {
	TemplateCode    string          `json:"templateCode"`
	TemplateVersion int             `json:"templateVersion"`
	SheetName       string          `json:"sheetName"`
	Fields          []TemplateField `json:"fields"`
	Rows            []ParsedRow     `json:"rows"`
}

type XLSXParser struct {
	config ParserConfig
}

var attendanceImportFields = []TemplateField{
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

// NewAttendanceXLSXParser returns the locked first-version attendance template parser.
func NewAttendanceXLSXParser(maxCompressedBytes int64) *XLSXParser {
	return NewXLSXParser(ParserConfig{
		ExpectedTemplateCode: "ATTENDANCE_IMPORT", ExpectedTemplateVersion: 1,
		ExpectedSheetName: "到课率导入", Fields: append([]TemplateField(nil), attendanceImportFields...), DataStartRow: 5,
		Limits: XLSXLimits{MaxCompressedBytes: maxCompressedBytes, MaxEntries: 512, MaxEntryUncompressedBytes: 16 << 20, MaxTotalUncompressedBytes: 64 << 20, MaxRows: 1000},
	})
}

func NewXLSXParser(config ParserConfig) *XLSXParser {
	config.Limits = defaultXLSXLimits(config.Limits)
	if config.DataStartRow == 0 {
		config.DataStartRow = 5
	}
	return &XLSXParser{config: config}
}

// Parse performs ZIP safety checks before handing the in-memory OOXML package to Excelize.
func (p *XLSXParser) Parse(filename string, content []byte) (ParsedWorkbook, error) {
	if strings.ToLower(filepath.Ext(strings.TrimSpace(filename))) != ".xlsx" {
		return ParsedWorkbook{}, parseError(ErrorInvalidFileType, "仅支持无宏的 .xlsx 文件")
	}
	if int64(len(content)) > p.config.Limits.MaxCompressedBytes {
		return ParsedWorkbook{}, parseError(ErrorFileTooLarge, "文件大小超过限制")
	}
	if len(content) < 4 || !bytes.Equal(content[:4], []byte{'P', 'K', 0x03, 0x04}) {
		return ParsedWorkbook{}, parseError(ErrorInvalidXLSXSignature, "文件不是有效的 XLSX 压缩包")
	}
	if err := preflightXLSXArchive(content, p.config.Limits); err != nil {
		return ParsedWorkbook{}, err
	}

	book, err := excelize.OpenReader(bytes.NewReader(content), excelize.Options{
		UnzipSizeLimit:    excelizeLimit(p.config.Limits.MaxTotalUncompressedBytes),
		UnzipXMLSizeLimit: excelizeLimit(p.config.Limits.MaxEntryUncompressedBytes),
	})
	if err != nil {
		return ParsedWorkbook{}, parseError(ErrorInvalidWorkbook, "工作簿结构无效或无法读取")
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) != 1 || sheets[0] != p.config.ExpectedSheetName {
		return ParsedWorkbook{}, parseError(ErrorInvalidWorkbook, "工作簿必须且只能包含指定业务工作表")
	}
	rows, err := book.GetRows(sheets[0])
	if err != nil {
		return ParsedWorkbook{}, parseError(ErrorInvalidWorkbook, "无法读取业务工作表")
	}
	return p.parseRows(sheets[0], rows)
}

func excelizeLimit(value uint64) int64 {
	const maxInt64 = uint64(1<<63 - 1)
	if value > maxInt64 {
		return int64(maxInt64)
	}
	return int64(value)
}

func (p *XLSXParser) parseRows(sheetName string, rows [][]string) (ParsedWorkbook, error) {
	if len(rows) < p.config.DataStartRow-1 || len(p.config.Fields) == 0 || p.config.ExpectedTemplateCode == "" || p.config.ExpectedTemplateVersion < 1 {
		return ParsedWorkbook{}, parseError(ErrorInvalidWorkbook, "模板元数据或表头不完整")
	}
	metadata := normalizedRow(rows[0], 4)
	if metadata[0] != "template_code" || metadata[2] != "template_version" || metadata[1] != p.config.ExpectedTemplateCode {
		return ParsedWorkbook{}, parseError(ErrorInvalidWorkbook, "模板编码无效")
	}
	version, err := strconv.Atoi(metadata[3])
	if err != nil || version != p.config.ExpectedTemplateVersion {
		return ParsedWorkbook{}, parseError(ErrorTemplateVersionMismatch, "模板版本与当前版本不一致")
	}
	if !p.validHeaders(rows[1], rows[2]) {
		return ParsedWorkbook{}, parseError(ErrorInvalidHeader, "模板字段或中文表头已被修改")
	}

	result := ParsedWorkbook{
		TemplateCode: metadata[1], TemplateVersion: version, SheetName: sheetName,
		Fields: append([]TemplateField(nil), p.config.Fields...), Rows: make([]ParsedRow, 0),
	}
	for index := p.config.DataStartRow - 1; index < len(rows); index++ {
		values := normalizedRow(rows[index], len(p.config.Fields))
		if blankRow(values) {
			continue
		}
		if len(result.Rows) >= p.config.Limits.MaxRows {
			return ParsedWorkbook{}, parseError(ErrorRowLimitExceeded, "业务数据超过最大行数")
		}
		parsed := ParsedRow{RowNo: index + 1, Values: make(map[string]string, len(p.config.Fields))}
		for fieldIndex, field := range p.config.Fields {
			parsed.Values[field.Code] = values[fieldIndex]
		}
		result.Rows = append(result.Rows, parsed)
	}
	return result, nil
}

func (p *XLSXParser) validHeaders(codes, headers []string) bool {
	if hasExtraValues(codes, len(p.config.Fields)) || hasExtraValues(headers, len(p.config.Fields)) {
		return false
	}
	normalizedCodes := normalizedRow(codes, len(p.config.Fields))
	normalizedHeaders := normalizedRow(headers, len(p.config.Fields))
	for index, field := range p.config.Fields {
		if normalizedCodes[index] != field.Code || normalizedHeaders[index] != field.Header {
			return false
		}
	}
	return true
}

func preflightXLSXArchive(content []byte, limits XLSXLimits) error {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return parseError(ErrorInvalidXLSXSignature, "XLSX 压缩包目录无效")
	}
	if len(reader.File) == 0 || len(reader.File) > limits.MaxEntries {
		return parseError(ErrorXLSXArchiveLimitExceeded, "XLSX 压缩条目数量超过限制")
	}
	seen := make(map[string]struct{}, len(reader.File))
	var totalUncompressed uint64
	for _, file := range reader.File {
		if unsafeArchiveName(file.Name) || file.Mode()&os.ModeSymlink != 0 {
			return parseError(ErrorXLSXArchiveLimitExceeded, "XLSX 包含不安全的压缩路径")
		}
		if _, exists := seen[file.Name]; exists {
			return parseError(ErrorXLSXArchiveLimitExceeded, "XLSX 包含重复压缩条目")
		}
		seen[file.Name] = struct{}{}
		lowerName := strings.ToLower(file.Name)
		if strings.Contains(lowerName, "vbaproject") || strings.HasPrefix(lowerName, "xl/macrosheets/") {
			return parseError(ErrorInvalidFileType, "不允许包含宏的工作簿")
		}
		if file.Flags&0x1 != 0 {
			return parseError(ErrorInvalidFileType, "不允许加密的工作簿")
		}
		if file.UncompressedSize64 > limits.MaxEntryUncompressedBytes {
			return parseError(ErrorXLSXArchiveLimitExceeded, "XLSX 解压大小超过限制")
		}
		if totalUncompressed > limits.MaxTotalUncompressedBytes || file.UncompressedSize64 > limits.MaxTotalUncompressedBytes-totalUncompressed {
			return parseError(ErrorXLSXArchiveLimitExceeded, "XLSX 解压总量超过限制")
		}
		totalUncompressed += file.UncompressedSize64
	}
	contentTypes, ok := zipEntry(reader.File, "[Content_Types].xml")
	if !ok {
		return parseError(ErrorInvalidWorkbook, "XLSX 缺少内容类型声明")
	}
	declaration, err := readLimitedZipEntry(contentTypes, limits.MaxEntryUncompressedBytes)
	if err != nil {
		return parseError(ErrorXLSXArchiveLimitExceeded, "无法安全读取 XLSX 内容类型")
	}
	lowerDeclaration := strings.ToLower(string(declaration))
	// Excelize declares a generic .bin content type even for macro-free files. An actual
	// vbaProject part is rejected above; only a macro-enabled workbook declaration is fatal here.
	if strings.Contains(lowerDeclaration, "macroenabled") {
		return parseError(ErrorInvalidFileType, "不允许包含宏的工作簿")
	}
	return nil
}

func defaultXLSXLimits(limits XLSXLimits) XLSXLimits {
	if limits.MaxCompressedBytes <= 0 {
		limits.MaxCompressedBytes = 10 << 20
	}
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = 256
	}
	if limits.MaxEntryUncompressedBytes == 0 {
		limits.MaxEntryUncompressedBytes = 16 << 20
	}
	if limits.MaxTotalUncompressedBytes == 0 {
		limits.MaxTotalUncompressedBytes = 64 << 20
	}
	if limits.MaxRows <= 0 {
		limits.MaxRows = 1000
	}
	return limits
}

func unsafeArchiveName(name string) bool {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) {
		return true
	}
	cleaned := path.Clean(name)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != strings.TrimSuffix(name, "/")
}

func zipEntry(files []*zip.File, name string) (*zip.File, bool) {
	for _, file := range files {
		if file.Name == name {
			return file, true
		}
	}
	return nil, false
}

func readLimitedZipEntry(file *zip.File, limit uint64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, int64(limit)+1)
	content, err := io.ReadAll(limited)
	if err != nil || uint64(len(content)) > limit {
		return nil, fmt.Errorf("zip entry exceeds limit")
	}
	return content, nil
}

func normalizedRow(row []string, width int) []string {
	result := make([]string, width)
	for index := 0; index < width && index < len(row); index++ {
		result[index] = strings.TrimSpace(row[index])
	}
	return result
}

func hasExtraValues(row []string, width int) bool {
	for index := width; index < len(row); index++ {
		if strings.TrimSpace(row[index]) != "" {
			return true
		}
	}
	return false
}

func blankRow(values []string) bool {
	for _, value := range values {
		if value != "" {
			return false
		}
	}
	return true
}

func parseError(code ErrorCode, message string) error {
	return &ParseError{Code: code, Message: message}
}
