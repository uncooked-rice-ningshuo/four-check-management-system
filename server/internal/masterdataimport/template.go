package masterdataimport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
	"github.com/xuri/excelize/v2"
)

const templateVersion = 1

// FieldSpec is both the locked workbook contract and the human-readable validation rule.
type FieldSpec struct {
	Code     string `json:"code"`
	Header   string `json:"header"`
	Required bool   `json:"required"`
	Format   string `json:"format"`
	MaxRunes int    `json:"maxRunes,omitempty"`
}

type TemplateSpec struct {
	Kind            string      `json:"kind"`
	TemplateCode    string      `json:"templateCode"`
	TemplateVersion int         `json:"templateVersion"`
	SheetName       string      `json:"sheetName"`
	Label           string      `json:"label"`
	Fields          []FieldSpec `json:"fields"`
}

var templateSpecs = []TemplateSpec{
	{Kind: masterdata.KindFaculty, TemplateCode: "MASTER_DATA_FACULTIES", TemplateVersion: templateVersion, SheetName: "院系导入", Label: "院系", Fields: []FieldSpec{
		{Code: "name", Header: "院系名称", Required: true, MaxRunes: 96, Format: "必填，去除首尾空格后 1–96 个字符；必须与现有院系名称不同。"},
	}},
	{Kind: masterdata.KindGrade, TemplateCode: "MASTER_DATA_GRADES", TemplateVersion: templateVersion, SheetName: "年级导入", Label: "年级", Fields: []FieldSpec{
		{Code: "year", Header: "入学年份", Required: true, Format: "必填，4 位 ASCII 数字，例如 2026；系统保存为“2026级”。"},
	}},
	{Kind: masterdata.KindMajor, TemplateCode: "MASTER_DATA_MAJORS", TemplateVersion: templateVersion, SheetName: "专业导入", Label: "专业", Fields: []FieldSpec{
		{Code: "faculty_name", Header: "所属院系名称", Required: true, MaxRunes: 96, Format: "必填；须与已启用院系名称完全一致。"},
		{Code: "name", Header: "专业名称", Required: true, MaxRunes: 96, Format: "必填，1–96 个字符；同一院系内不能重名。"},
	}},
	{Kind: masterdata.KindStaff, TemplateCode: "MASTER_DATA_STAFF", TemplateVersion: templateVersion, SheetName: "教职工导入", Label: "教职工", Fields: []FieldSpec{
		{Code: "employee_no", Header: "工号", Required: true, MaxRunes: 32, Format: "必填，文本，1–32 个字符；系统按大写规范化且工号必须唯一。请将 Excel 单元格设为文本以保留前导零。"},
		{Code: "name", Header: "姓名", Required: true, MaxRunes: 64, Format: "必填，1–64 个字符。"},
		{Code: "gender", Header: "性别", MaxRunes: 16, Format: "选填，最多16个字符；填写当前启用的 GENDER 字典值；留空默认 UNKNOWN。"},
		{Code: "phone", Header: "联系电话", MaxRunes: 32, Format: "选填，最多 32 个字符；按文本处理，不校验固定号码格式。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "选填；填写当前已启用院系的完整名称。"},
	}},
	{Kind: masterdata.KindClass, TemplateCode: "MASTER_DATA_CLASSES", TemplateVersion: templateVersion, SheetName: "班级导入", Label: "班级", Fields: []FieldSpec{
		{Code: "name", Header: "班级名称", Required: true, MaxRunes: 96, Format: "必填，1–96 个字符；班级名称不能与现有班级重复。"},
		{Code: "grade_year", Header: "入学年份", Required: true, Format: "必填，4 位 ASCII 数字，例如 2026；须对应已启用年级。"},
		{Code: "enrolled_size", Header: "实际在籍人数", Required: true, Format: "必填，十进制数字组成的 32 位非负整数，范围 0–2147483647；不能有正负号、小数或千位分隔符。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "选填；填写当前已启用院系的完整名称。"},
		{Code: "major_name", Header: "专业名称", MaxRunes: 96, Format: "选填；须与当前已启用专业匹配；填写专业时若同名专业跨院系，须同时填写所属院系。"},
		{Code: "head_teacher_employee_no", Header: "班主任工号", MaxRunes: 32, Format: "选填；填写当前已启用教职工的工号，按不区分大小写方式匹配。"},
	}},
	{Kind: masterdata.KindDormitoryArea, TemplateCode: "MASTER_DATA_DORMITORY_AREAS", TemplateVersion: templateVersion, SheetName: "宿舍区导入", Label: "宿舍区", Fields: []FieldSpec{
		{Code: "name", Header: "宿舍区名称", Required: true, MaxRunes: 96, Format: "必填，1–96 个字符；名称不能与现有宿舍区重复。"},
	}},
	{Kind: masterdata.KindDormitoryBuilding, TemplateCode: "MASTER_DATA_DORMITORY_BUILDINGS", TemplateVersion: templateVersion, SheetName: "宿舍楼导入", Label: "宿舍楼", Fields: []FieldSpec{
		{Code: "area_name", Header: "所属宿舍区名称", Required: true, MaxRunes: 96, Format: "必填；须与已启用宿舍区名称完全一致。"},
		{Code: "name", Header: "宿舍楼名称", Required: true, MaxRunes: 96, Format: "必填，1–96 个字符；同一宿舍区内不能重名。"},
	}},
	{Kind: masterdata.KindDormitory, TemplateCode: "MASTER_DATA_DORMITORIES", TemplateVersion: templateVersion, SheetName: "宿舍导入", Label: "宿舍", Fields: []FieldSpec{
		{Code: "area_name", Header: "宿舍区名称", Required: true, MaxRunes: 96, Format: "必填；须与已启用宿舍区名称完全一致。"},
		{Code: "building_name", Header: "宿舍楼名称", Required: true, MaxRunes: 96, Format: "必填；与宿舍区组合后须唯一匹配已启用宿舍楼。"},
		{Code: "room_no", Header: "宿舍号", Required: true, MaxRunes: 32, Format: "必填，文本，1–32 个字符；同一宿舍楼内不能重复。请将 Excel 单元格设为文本以保留前导零。"},
	}},
	{Kind: masterdata.KindTeachingBuilding, TemplateCode: "MASTER_DATA_TEACHING_BUILDINGS", TemplateVersion: templateVersion, SheetName: "教学楼导入", Label: "教学楼", Fields: []FieldSpec{
		{Code: "name", Header: "教学楼名称", Required: true, MaxRunes: 64, Format: "必填，1–64 个字符；名称不能与现有教学楼重复。"},
	}},
	{Kind: masterdata.KindClassroom, TemplateCode: "MASTER_DATA_CLASSROOMS", TemplateVersion: templateVersion, SheetName: "教室导入", Label: "教室", Fields: []FieldSpec{
		{Code: "building_name", Header: "教学楼名称", Required: true, MaxRunes: 64, Format: "必填；须与已启用教学楼名称完全一致。"},
		{Code: "room_no", Header: "教室号", Required: true, MaxRunes: 32, Format: "必填，文本，1–32 个字符；同一教学楼内不能重复。请将 Excel 单元格设为文本以保留前导零。"},
	}},
	{Kind: masterdata.KindStudent, TemplateCode: "MASTER_DATA_STUDENTS", TemplateVersion: templateVersion, SheetName: "学生导入", Label: "学生", Fields: []FieldSpec{
		{Code: "student_no", Header: "学号", Required: true, MaxRunes: 32, Format: "必填，文本，1–32 个字符；系统按大写规范化且学号必须唯一。请将 Excel 单元格设为文本以保留前导零。"},
		{Code: "name", Header: "姓名", Required: true, MaxRunes: 64, Format: "必填，1–64 个字符。"},
		{Code: "grade_year", Header: "入学年份", Required: true, Format: "必填，4 位 ASCII 数字，例如 2026；须对应已启用年级。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "选填；填写当前已启用院系的完整名称。"},
		{Code: "major_name", Header: "专业名称", MaxRunes: 96, Format: "选填；须与当前已启用专业匹配；填写专业时若同名专业跨院系，须同时填写所属院系。"},
		{Code: "class_name", Header: "班级名称", MaxRunes: 96, Format: "选填；须与当前已启用班级名称完全一致，且该班级年级须与本行入学年份一致。"},
		{Code: "dormitory_area_name", Header: "宿舍区名称", MaxRunes: 96, Format: "选填；宿舍区、宿舍楼、宿舍号须同时填写或同时留空。"},
		{Code: "dormitory_building_name", Header: "宿舍楼名称", MaxRunes: 96, Format: "选填；须与宿舍区组合后唯一匹配当前已启用宿舍楼。"},
		{Code: "room_no", Header: "宿舍号", MaxRunes: 32, Format: "选填，文本，1–32 个字符；须与宿舍区和宿舍楼组合后匹配当前已启用宿舍。"},
		{Code: "gender", Header: "性别", MaxRunes: 16, Format: "选填，最多16个字符；填写当前启用的 GENDER 字典值；留空默认 UNKNOWN。"},
		{Code: "enrollment_status", Header: "学籍状态", MaxRunes: 24, Format: "选填，最多24个字符；填写当前启用的 ENROLLMENT_STATUS 字典值；留空默认 ENROLLED。"},
	}},
}

func Specs() []TemplateSpec {
	result := make([]TemplateSpec, len(templateSpecs))
	for index, spec := range templateSpecs {
		result[index] = spec
		result[index].Fields = append([]FieldSpec(nil), spec.Fields...)
	}
	return result
}

func SpecFor(kind string) (TemplateSpec, bool) {
	for _, spec := range templateSpecs {
		if spec.Kind == kind {
			copy := spec
			copy.Fields = append([]FieldSpec(nil), spec.Fields...)
			return copy, true
		}
	}
	return TemplateSpec{}, false
}

func NewParser(kind string, maxCompressedBytes int64) (*importjob.XLSXParser, error) {
	spec, ok := SpecFor(kind)
	if !ok || maxCompressedBytes <= 0 {
		return nil, ErrInvalidKind
	}
	fields := make([]importjob.TemplateField, len(spec.Fields))
	for index, field := range spec.Fields {
		fields[index] = importjob.TemplateField{Code: field.Code, Header: field.Header}
	}
	return importjob.NewXLSXParser(importjob.ParserConfig{
		ExpectedTemplateCode: spec.TemplateCode, ExpectedTemplateVersion: spec.TemplateVersion,
		ExpectedSheetName: spec.SheetName, Fields: fields, DataStartRow: 5,
		Limits: importjob.XLSXLimits{MaxCompressedBytes: maxCompressedBytes, MaxEntries: 512,
			MaxEntryUncompressedBytes: 16 << 20, MaxTotalUncompressedBytes: 64 << 20, MaxRows: 1000},
	}), nil
}

func TemplateXLSX(kind string) ([]byte, error) {
	spec, ok := SpecFor(kind)
	if !ok {
		return nil, ErrInvalidKind
	}
	fields := make([]importjob.TemplateField, len(spec.Fields))
	for index, field := range spec.Fields {
		fields[index] = importjob.TemplateField{Code: field.Code, Header: field.Header}
	}
	lastColumn, err := excelize.ColumnNumberToName(len(fields))
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", spec.SheetName); err != nil {
		return nil, err
	}
	rows := [][]any{{"template_code", spec.TemplateCode, "template_version", spec.TemplateVersion}, {}, {}, {templateInstructions(spec)}}
	for _, field := range fields {
		rows[1] = append(rows[1], field.Code)
		rows[2] = append(rows[2], field.Header)
	}
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			return nil, err
		}
		if err := book.SetSheetRow(spec.SheetName, cell, &row); err != nil {
			return nil, err
		}
	}
	if err := book.SetRowVisible(spec.SheetName, 1, false); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(spec.SheetName, 2, false); err != nil {
		return nil, err
	}
	if err := book.MergeCell(spec.SheetName, "A4", lastColumn+"4"); err != nil {
		return nil, err
	}
	if err := book.SetColWidth(spec.SheetName, "A", lastColumn, 22); err != nil {
		return nil, err
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(spec.SheetName, "A5", lastColumn+"1004", textStyle); err != nil {
		return nil, err
	}
	if err := book.SetPanes(spec.SheetName, &excelize.Panes{Freeze: true, YSplit: 4, TopLeftCell: "A5", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return canonicalizeWorkbook(buffer.Bytes())
}

func templateInstructions(spec TemplateSpec) string {
	rules := make([]string, 0, len(spec.Fields)+1)
	rules = append(rules, "新增导入：只填写第5行及之后；保留前四行、字段顺序和工作表名称；所有字段按文本填写；不能包含公式；任一行校验失败时整批不能导入。")
	for _, field := range spec.Fields {
		rules = append(rules, field.Header+"："+field.Format)
	}
	return strings.Join(rules, "  ")
}

func canonicalizeWorkbook(content []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
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
		fileContent, readErr := io.ReadAll(input)
		closeErr := input.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		output, err := writer.Create(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, err := output.Write(fileContent); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close template archive: %w", err)
	}
	return result.Bytes(), nil
}
