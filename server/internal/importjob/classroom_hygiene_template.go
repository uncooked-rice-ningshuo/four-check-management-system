package importjob

// Hygiene result and responsibility columns stay identical across object templates.
var classroomHygieneImportFields = append([]TemplateField{{Code: "classroom_name", Header: "教室完整名称"}}, dormitoryHygieneImportFields[1:]...)

// BuildClassroomHygieneTemplate identifies one configured classroom area per check.
func BuildClassroomHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("教室卫生导入", "CLASSROOM_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每间教室每天只录入一条，不支持复查。教室填写教学楼与教室号完整名称，日期YYYY-MM-DD。结果填优秀/合格/不合格；不合格须填写情况说明。每条记录填写一名责任班级和一名责任学生。", classroomHygieneImportFields)
}
