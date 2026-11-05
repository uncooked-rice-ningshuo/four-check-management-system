package importjob

// Classroom hygiene keeps the shared hygiene result and responsibility columns
// but orders the responsible class before the student name and number.
var classroomHygieneImportFields = []TemplateField{
	{Code: "classroom_name", Header: "教室"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "result", Header: "检查结果"},
	{Code: "responsible_class_names", Header: "责任班级"},
	{Code: "responsible_student_name", Header: "责任学生姓名"},
	{Code: "responsible_student_no", Header: "责任学生学号"},
	{Code: "description", Header: "情况说明"},
	{Code: "remark", Header: "备注"},
}

// BuildClassroomHygieneTemplate identifies one configured classroom per check.
func BuildClassroomHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("教室卫生导入", "CLASSROOM_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每间教室每天只录入一条，不支持复查。教室填写教学楼与教室号完整名称，日期YYYY-MM-DD。结果填优秀/合格/不合格；不合格须填写情况说明。每条记录填写一名责任班级和一名责任学生。", classroomHygieneImportFields)
}
