package importjob

// Each dormitory hygiene row has one responsible class and student.
var dormitoryHygieneImportFields = []TemplateField{
	{Code: "dormitory_name", Header: "宿舍完整名称"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "result", Header: "检查结果"},
	{Code: "responsible_class_names", Header: "责任班级"},
	{Code: "responsible_student_no", Header: "责任学生学号"},
	{Code: "description", Header: "情况说明"},
	{Code: "remark", Header: "备注"},
}

// BuildDormitoryHygieneTemplate uses system names instead of internal UUIDs.
func BuildDormitoryHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("宿舍卫生导入", "DORMITORY_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每间宿舍每天只录入一条，不支持复查。日期YYYY-MM-DD。结果填优秀/合格/不合格；不合格须填写情况说明。每条记录填写一名责任班级和一名责任学生。情况说明为自由文本，可在系统录入时使用短语起稿后修改。", dormitoryHygieneImportFields)
}
