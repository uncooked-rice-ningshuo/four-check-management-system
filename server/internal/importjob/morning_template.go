package importjob

// Morning templates contain one student/day. The server owns absenceCount=1;
// no time, exercise/sign-in subtype or cumulative count is accepted as input.
var morningImportFields = []TemplateField{
	{Code: "student_no", Header: "学号"},
	{Code: "student_name", Header: "姓名"},
	{Code: "class_name", Header: "班级"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "remark", Header: "备注"},
}

// BuildMorningTemplate generates the version-locked student/day import workbook.
func BuildMorningTemplate() ([]byte, error) {
	return buildImportTemplate("早锻炼早签导入", "MORNING_ATTENDANCE_IMPORT", "填写说明：保留前四行，从第5行填写；日期YYYY-MM-DD。每人每天一条，缺勤次数由系统固定为1；不填写累计次数，不区分早锻炼与早签。", morningImportFields)
}

// NewMorningXLSXParser reuses ZIP, XML, formula and row-limit protection.
func NewMorningXLSXParser(maxCompressedBytes int64) *XLSXParser {
	config := NewAttendanceXLSXParser(maxCompressedBytes).config
	config.ExpectedTemplateCode = "MORNING_ATTENDANCE_IMPORT"
	config.ExpectedSheetName = "早锻炼早签导入"
	config.Fields = append([]TemplateField(nil), morningImportFields...)
	return NewXLSXParser(config)
}
