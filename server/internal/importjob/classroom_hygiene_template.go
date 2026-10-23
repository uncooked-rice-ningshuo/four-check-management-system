package importjob

// Hygiene result and responsibility columns stay identical across object templates.
var classroomHygieneImportFields = append([]TemplateField{{Code: "classroom_name", Header: "教室完整名称"}, {Code: "area_name", Header: "检查区域名称"}}, dormitoryHygieneImportFields[1:]...)

// BuildClassroomHygieneTemplate identifies one configured classroom area per check.
func BuildClassroomHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("教室卫生导入", "CLASSROOM_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每个教室区域每次检查一条。教室填写教学楼与教室号完整名称，区域必须属于该教室。日期YYYY-MM-DD；时间HH:mm:ss与轮次二选一。结果填优秀/合格/不合格；不合格须填写已配置原因名称。责任班级必填，多项用分号分隔；责任学生学号可空。不填写累计次数。", classroomHygieneImportFields)
}
