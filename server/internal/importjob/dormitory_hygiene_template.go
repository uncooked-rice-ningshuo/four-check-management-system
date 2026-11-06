package importjob

// Each dormitory hygiene row has one responsible class and student.
// The reason column stays free text; dictionary phrases are a typing aid only.
var dormitoryHygieneImportFields = []TemplateField{
	{Code: "dormitory_name", Header: "宿舍"},
	{Code: "responsible_student_no", Header: "责任学生学号"},
	{Code: "responsible_student_name", Header: "责任学生姓名"},
	{Code: "responsible_class_names", Header: "责任学生班级"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "result", Header: "检查结果"},
	{Code: "description", Header: "原因"},
	{Code: "remark", Header: "备注"},
}

// BuildDormitoryHygieneTemplate uses system names instead of internal UUIDs.
func BuildDormitoryHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("宿舍卫生导入", "DORMITORY_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每间宿舍每天只录入一条，不支持复查。日期YYYY-MM-DD。结果填优秀/合格/不合格；不合格须填写原因。每条记录填写一名责任学生班级、姓名和学号。原因为自由文本，不限定字典词条。", dormitoryHygieneImportFields)
}
