package importjob

// Late-return rows represent one student event, with a fully qualified dormitory.
var lateReturnImportFields = []TemplateField{
	{Code: "student_no", Header: "学号"}, {Code: "student_name", Header: "姓名"}, {Code: "class_name", Header: "班级"},
	{Code: "inspection_date", Header: "晚归日期"}, {Code: "inspection_time", Header: "晚归时间"},
	{Code: "dormitory_name", Header: "宿舍完整名称"}, {Code: "remark", Header: "备注"},
}

// BuildLateReturnTemplate preserves individual events, never cumulative counts.
func BuildLateReturnTemplate() ([]byte, error) {
	return buildImportTemplate("晚归导入", "LATE_RETURN_IMPORT", "填写说明：保留前四行，从第5行填写。每名学生每次晚归一行；日期YYYY-MM-DD、时间HH:mm或HH:mm:ss；宿舍填写系统中的完整名称（宿舍区 楼栋 房间号）。不填写累计次数。", lateReturnImportFields)
}

// NewLateReturnXLSXParser enforces the same workbook safety policy as other types.
func NewLateReturnXLSXParser(maxBytes int64) *XLSXParser {
	config := NewAttendanceXLSXParser(maxBytes).config
	config.ExpectedTemplateCode = "LATE_RETURN_IMPORT"
	config.ExpectedSheetName = "晚归导入"
	config.Fields = append([]TemplateField(nil), lateReturnImportFields...)
	return NewXLSXParser(config)
}
