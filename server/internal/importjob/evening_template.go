package importjob

// Evening self-study import leaves the fixed period and one absence incident to
// the server; no classroom and no time is collected.
var eveningSelfStudyImportFields = []TemplateField{
	{Code: "student_no", Header: "学号"},
	{Code: "student_name", Header: "姓名"},
	{Code: "class_name", Header: "班级"},
	{Code: "absence_date", Header: "缺勤日期"},
	{Code: "remark", Header: "备注"},
}

// BuildEveningSelfStudyTemplate generates the version-locked import workbook.
func BuildEveningSelfStudyTemplate() ([]byte, error) {
	return buildImportTemplate("晚自习导入", "EVENING_SELF_STUDY_IMPORT", "填写说明：每名缺勤学生每天一条记录；日期使用 YYYY-MM-DD。节次固定为第 9–10 节，每次缺勤计 1 次并按周倍数计入违纪，由系统计算；不要填写教室和到场学生。", eveningSelfStudyImportFields)
}
