package masterdataimport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
	"github.com/xuri/excelize/v2"
)

const templateVersion = 4

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
	ExampleRow      []string    `json:"-"`
}

var templateSpecs = []TemplateSpec{
	{Kind: masterdata.KindFaculty, TemplateCode: "MASTER_DATA_FACULTIES", TemplateVersion: templateVersion, SheetName: "院系导入", Label: "院系", ExampleRow: []string{"计算机科学与工程学院"}, Fields: []FieldSpec{
		{Code: "name", Header: "院系名称", Required: true, MaxRunes: 96, Format: "文本；必填，去除首尾空格后 1–96 个字符；必须与现有院系名称不同。"},
	}},
	{Kind: masterdata.KindGrade, TemplateCode: "MASTER_DATA_GRADES", TemplateVersion: templateVersion, SheetName: "年级导入", Label: "年级", ExampleRow: []string{"2026"}, Fields: []FieldSpec{
		{Code: "year", Header: "入学年份", Required: true, Format: "数字；必填，4 位 ASCII 数字，例如 2026；系统保存为“2026级”。"},
	}},
	{Kind: masterdata.KindMajor, TemplateCode: "MASTER_DATA_MAJORS", TemplateVersion: templateVersion, SheetName: "专业导入", Label: "专业", ExampleRow: []string{"计算机科学与工程学院", "软件工程"}, Fields: []FieldSpec{
		{Code: "faculty_name", Header: "所属院系名称", Required: true, MaxRunes: 96, Format: "文本；必填；须与已启用院系名称完全一致。"},
		{Code: "name", Header: "专业名称", Required: true, MaxRunes: 96, Format: "文本；必填，1–96 个字符；同一院系内不能重名。"},
	}},
	{Kind: masterdata.KindStaff, TemplateCode: "MASTER_DATA_STAFF", TemplateVersion: templateVersion, SheetName: "教职工导入", Label: "教职工", ExampleRow: []string{"T2026001", "张老师", "男", "13800000000", "计算机科学与工程学院"}, Fields: []FieldSpec{
		{Code: "employee_no", Header: "工号", Required: true, MaxRunes: 32, Format: "文本；必填，1–32 个字符；系统按大写规范化且工号必须唯一。请将 Excel 单元格设为文本以保留前导零。"},
		{Code: "name", Header: "姓名", Required: true, MaxRunes: 64, Format: "文本；必填，1–64 个字符。"},
		{Code: "gender", Header: "性别", MaxRunes: 16, Format: "枚举（字典）；选填，最多 16 个字符；当前启用项可填显示名称（如“男”或“女”）或选项值（如 MALE 或 FEMALE）；留空默认 UNKNOWN。"},
		{Code: "phone", Header: "联系电话", MaxRunes: 32, Format: "文本；选填，最多 32 个字符；不校验固定号码格式。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "文本；选填；填写当前已启用院系的完整名称。"},
	}},
	{Kind: masterdata.KindClass, TemplateCode: "MASTER_DATA_CLASSES", TemplateVersion: templateVersion, SheetName: "班级导入", Label: "班级", ExampleRow: []string{"2026级软件工程一班", "2026", "32", "计算机科学与工程学院", "软件工程", "T2026001"}, Fields: []FieldSpec{
		{Code: "name", Header: "班级名称", Required: true, MaxRunes: 96, Format: "文本；必填，1–96 个字符；班级名称不能与现有班级重复。"},
		{Code: "grade_year", Header: "入学年份", Required: true, Format: "数字；必填，4 位 ASCII 数字，例如 2026；须对应已启用年级。"},
		{Code: "enrolled_size", Header: "实际在籍人数", Required: true, Format: "整数；必填，十进制非负整数，范围 0–2147483647；不能有正负号、小数或千位分隔符。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "文本；选填；填写当前已启用院系的完整名称。"},
		{Code: "major_name", Header: "专业名称", MaxRunes: 96, Format: "文本；选填；须与当前已启用专业匹配；填写专业时若同名专业跨院系，须同时填写所属院系。"},
		{Code: "head_teacher_employee_no", Header: "班主任工号", MaxRunes: 32, Format: "文本；选填；填写当前已启用教职工的工号，按不区分大小写方式匹配。"},
	}},
	{Kind: masterdata.KindDormitoryArea, TemplateCode: "MASTER_DATA_DORMITORY_AREAS", TemplateVersion: templateVersion, SheetName: "宿舍区导入", Label: "宿舍区", ExampleRow: []string{"4 区"}, Fields: []FieldSpec{
		{Code: "name", Header: "宿舍区名称", Required: true, MaxRunes: 96, Format: "文本；必填，1–96 个字符；名称不能与现有宿舍区重复。"},
	}},
	{Kind: masterdata.KindDormitoryBuilding, TemplateCode: "MASTER_DATA_DORMITORY_BUILDINGS", TemplateVersion: templateVersion, SheetName: "宿舍楼导入", Label: "宿舍楼", ExampleRow: []string{"4 区", "1栋"}, Fields: []FieldSpec{
		{Code: "area_name", Header: "所属宿舍区名称", Required: true, MaxRunes: 96, Format: "文本；必填；须与已启用宿舍区名称完全一致。"},
		{Code: "name", Header: "宿舍楼名称", Required: true, MaxRunes: 96, Format: "文本；必填，1–96 个字符；同一宿舍区内不能重名。"},
	}},
	{Kind: masterdata.KindDormitory, TemplateCode: "MASTER_DATA_DORMITORIES", TemplateVersion: templateVersion, SheetName: "宿舍导入", Label: "宿舍", ExampleRow: []string{"4 区", "1栋", "001"}, Fields: []FieldSpec{
		{Code: "area_name", Header: "宿舍区名称", Required: true, MaxRunes: 96, Format: "文本；必填；须与已启用宿舍区名称完全一致。"},
		{Code: "building_name", Header: "宿舍楼名称", Required: true, MaxRunes: 96, Format: "文本；必填；与宿舍区组合后须唯一匹配已启用宿舍楼。"},
		{Code: "room_no", Header: "宿舍号", Required: true, MaxRunes: 32, Format: "文本；必填，1–32 个字符；同一宿舍楼内不能重复。请将 Excel 单元格设为文本以保留前导零。"},
	}},
	{Kind: masterdata.KindTeachingBuilding, TemplateCode: "MASTER_DATA_TEACHING_BUILDINGS", TemplateVersion: templateVersion, SheetName: "教学楼导入", Label: "教学楼", ExampleRow: []string{"第一教学楼"}, Fields: []FieldSpec{
		{Code: "name", Header: "教学楼名称", Required: true, MaxRunes: 64, Format: "文本；必填，1–64 个字符；名称不能与现有教学楼重复。"},
	}},
	{Kind: masterdata.KindClassroom, TemplateCode: "MASTER_DATA_CLASSROOMS", TemplateVersion: templateVersion, SheetName: "教室导入", Label: "教室", ExampleRow: []string{"第一教学楼", "101"}, Fields: []FieldSpec{
		{Code: "building_name", Header: "教学楼名称", Required: true, MaxRunes: 64, Format: "文本；必填；须与已启用教学楼名称完全一致。"},
		{Code: "room_no", Header: "教室号", Required: true, MaxRunes: 32, Format: "文本；必填，1–32 个字符；同一教学楼内不能重复。请将 Excel 单元格设为文本以保留前导零。"},
	}},
	{Kind: masterdata.KindStudent, TemplateCode: "MASTER_DATA_STUDENTS", TemplateVersion: templateVersion, SheetName: "学生导入", Label: "学生", ExampleRow: []string{"2000000000", "张同学", "2026", "计算机科学与工程学院", "软件工程", "2026级软件工程一班", "4 区", "1栋", "001", "男", "在籍"}, Fields: []FieldSpec{
		{Code: "student_no", Header: "学号", Required: true, MaxRunes: 32, Format: "文本；必填，1–32 个字符；系统按大写规范化且学号必须唯一。请将 Excel 单元格设为文本以保留前导零。"},
		{Code: "name", Header: "姓名", Required: true, MaxRunes: 64, Format: "文本；必填，1–64 个字符。"},
		{Code: "grade_year", Header: "入学年份", Required: true, Format: "数字；必填，4 位 ASCII 数字，例如 2026；须对应已启用年级。"},
		{Code: "faculty_name", Header: "所属院系名称", MaxRunes: 96, Format: "文本；选填；填写当前已启用院系的完整名称。"},
		{Code: "major_name", Header: "专业名称", MaxRunes: 96, Format: "文本；选填；须与当前已启用专业匹配；填写专业时若同名专业跨院系，须同时填写所属院系。"},
		{Code: "class_name", Header: "班级名称", MaxRunes: 96, Format: "文本；选填；须与当前已启用班级名称完全一致，且该班级年级须与本行入学年份一致。"},
		{Code: "dormitory_area_name", Header: "宿舍区名称", MaxRunes: 96, Format: "文本；选填；宿舍区、宿舍楼、宿舍号须同时填写或同时留空。"},
		{Code: "dormitory_building_name", Header: "宿舍楼名称", MaxRunes: 96, Format: "文本；选填；须与宿舍区组合后唯一匹配当前已启用宿舍楼。"},
		{Code: "room_no", Header: "宿舍号", MaxRunes: 32, Format: "文本；选填，1–32 个字符；须与宿舍区和宿舍楼组合后匹配当前已启用宿舍。"},
		{Code: "gender", Header: "性别", MaxRunes: 16, Format: "枚举（字典）；选填，最多 16 个字符；当前启用项可填显示名称（如“男”或“女”）或选项值（如 MALE 或 FEMALE）；留空默认 UNKNOWN。"},
		{Code: "enrollment_status", Header: "学籍状态", MaxRunes: 24, Format: "枚举（字典）；选填，最多 24 个字符；当前启用项可填显示名称（如“在籍”）或选项值（如 ENROLLED）；留空默认 ENROLLED。"},
	}},
}

func Specs() []TemplateSpec {
	result := make([]TemplateSpec, len(templateSpecs))
	for index, spec := range templateSpecs {
		result[index] = spec
		result[index].Fields = append([]FieldSpec(nil), spec.Fields...)
		result[index].ExampleRow = append([]string(nil), spec.ExampleRow...)
	}
	return result
}

func SpecFor(kind string) (TemplateSpec, bool) {
	for _, spec := range templateSpecs {
		if spec.Kind == kind {
			copy := spec
			copy.Fields = append([]FieldSpec(nil), spec.Fields...)
			copy.ExampleRow = append([]string(nil), spec.ExampleRow...)
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
		ExpectedSheetName: spec.SheetName, Fields: fields, DataStartRow: 6,
		Limits: importjob.XLSXLimits{MaxCompressedBytes: maxCompressedBytes, MaxEntries: 512,
			MaxEntryUncompressedBytes: 16 << 20, MaxTotalUncompressedBytes: 64 << 20, MaxRows: 1000},
	}), nil
}

func TemplateXLSX(kind string) ([]byte, error) {
	spec, ok := SpecFor(kind)
	if !ok {
		return nil, ErrInvalidKind
	}
	if len(spec.ExampleRow) != len(spec.Fields) {
		return nil, fmt.Errorf("template %s example row does not match its fields", spec.TemplateCode)
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
	rows := [][]any{{"template_code", spec.TemplateCode, "template_version", spec.TemplateVersion}, {}, {}, {}}
	for _, field := range fields {
		rows[1] = append(rows[1], field.Code)
		rows[2] = append(rows[2], field.Header)
	}
	for _, field := range spec.Fields {
		rows[3] = append(rows[3], field.Format)
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
	if err := book.SetSheetRow(spec.SheetName, "A5", &spec.ExampleRow); err != nil {
		return nil, err
	}
	if err := book.AddComment(spec.SheetName, excelize.Comment{
		Cell:      "A5",
		Author:    "基础数据导入",
		Paragraph: []excelize.RichTextRun{{Text: "第 4 行为各字段格式说明，第 5 行为示例且导入时自动忽略。请从第 6 行开始填写；关联示例值须替换为系统中已启用的数据。"}},
		Height:    60,
		Width:     280,
	}); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(spec.SheetName, 1, false); err != nil {
		return nil, err
	}
	if err := book.SetRowVisible(spec.SheetName, 2, false); err != nil {
		return nil, err
	}
	if err := book.SetColWidth(spec.SheetName, "A", lastColumn, 22); err != nil {
		return nil, err
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := book.SetCellStyle(spec.SheetName, "A6", lastColumn+"1005", textStyle); err != nil {
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
	if err := book.SetCellStyle(spec.SheetName, "A4", lastColumn+"4", formatStyle); err != nil {
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
	if err := book.SetCellStyle(spec.SheetName, "A5", lastColumn+"5", exampleStyle); err != nil {
		return nil, err
	}
	if err := book.SetRowHeight(spec.SheetName, 4, 110); err != nil {
		return nil, err
	}
	if err := book.SetRowHeight(spec.SheetName, 5, 24); err != nil {
		return nil, err
	}
	if err := book.SetPanes(spec.SheetName, &excelize.Panes{Freeze: true, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return canonicalizeWorkbook(buffer.Bytes())
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
