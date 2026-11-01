package importjob

// Public dormitory hygiene is attributed to a responsible dormitory.
var publicDormitoryHygieneImportFields = []TemplateField{
	{Code: "responsible_dormitory_name", Header: "责任宿舍完整名称"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "result", Header: "检查结果"},
	{Code: "responsible_class_names", Header: "责任班级"},
	{Code: "responsible_student_no", Header: "责任学生学号"},
	{Code: "description", Header: "情况说明"},
	{Code: "remark", Header: "备注"},
}

// BuildPublicDormitoryHygieneTemplate generates the public dormitory import workbook.
func BuildPublicDormitoryHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("公共宿舍卫生导入", "PUBLIC_DORMITORY_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每个责任宿舍每天只录入一条，不支持复查。此类型不记录公共区域，宿舍只表示责任归属。日期YYYY-MM-DD。不合格须填写情况说明。每条记录填写一名责任班级和一名责任学生。", publicDormitoryHygieneImportFields)
}

// NewPublicDormitoryHygieneXLSXParser validates the version-locked workbook contract.
func NewPublicDormitoryHygieneXLSXParser(maxBytes int64) *XLSXParser {
	config := NewAttendanceXLSXParser(maxBytes).config
	config.ExpectedTemplateCode = "PUBLIC_DORMITORY_HYGIENE_IMPORT"
	config.ExpectedSheetName = "公共宿舍卫生导入"
	config.Fields = append([]TemplateField(nil), publicDormitoryHygieneImportFields...)
	return NewXLSXParser(config)
}
