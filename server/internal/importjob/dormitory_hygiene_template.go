package importjob

// Dormitory hygiene is one object inspection per row; responsibility lists use
// semicolons, preserving students' leading zeroes and multi-class scope.
var dormitoryHygieneImportFields = []TemplateField{
	{Code: "dormitory_name", Header: "宿舍完整名称"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "inspection_time", Header: "检查时间"},
	{Code: "inspection_round", Header: "检查轮次"},
	{Code: "result", Header: "检查结果"},
	{Code: "responsible_class_names", Header: "责任班级"},
	{Code: "responsible_student_nos", Header: "责任学生学号"},
	{Code: "reason_name", Header: "原因名称"},
	{Code: "description", Header: "情况说明"},
	{Code: "remark", Header: "备注"},
}

// BuildDormitoryHygieneTemplate uses system names instead of internal UUIDs.
func BuildDormitoryHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("宿舍卫生导入", "DORMITORY_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每个宿舍每次检查一条。日期YYYY-MM-DD；时间HH:mm:ss与轮次二选一。结果填优秀/合格/不合格；不合格须填写已配置原因名称。责任班级必填，多项用分号分隔；责任学生学号可空。不填写累计次数。", dormitoryHygieneImportFields)
}
