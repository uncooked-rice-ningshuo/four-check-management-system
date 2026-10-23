package importjob

// The inspected public area remains separate from optional responsible rooms.
var publicDormitoryHygieneImportFields = []TemplateField{
	{Code: "area_name", Header: "公共区域完整名称"},
	{Code: "inspection_date", Header: "检查日期"},
	{Code: "inspection_time", Header: "检查时间"},
	{Code: "inspection_round", Header: "检查轮次"},
	{Code: "result", Header: "检查结果"},
	{Code: "responsible_class_names", Header: "责任班级"},
	{Code: "responsible_student_nos", Header: "责任学生学号"},
	{Code: "responsible_dormitory_names", Header: "责任宿舍完整名称"},
	{Code: "reason_name", Header: "原因名称"},
	{Code: "description", Header: "情况说明"},
	{Code: "remark", Header: "备注"},
}

// BuildPublicDormitoryHygieneTemplate defines draft fields without enabling the
// public import entry while the responsible-room scope decision is pending.
func BuildPublicDormitoryHygieneTemplate() ([]byte, error) {
	return buildImportTemplate("公共宿舍卫生导入", "PUBLIC_DORMITORY_HYGIENE_IMPORT", "填写说明：保留前四行，从第5行填写；每个公共区域每次检查一条。公共区域填写系统完整名称，责任宿舍与被检查区域分开填写。日期YYYY-MM-DD；时间HH:mm:ss与轮次二选一。结果填优秀/合格/不合格；不合格须填写已配置原因名称。责任班级必填，多项用分号分隔；责任学生和责任宿舍可空。不填写累计次数。", publicDormitoryHygieneImportFields)
}

// NewPublicDormitoryHygieneXLSXParser validates the draft workbook contract; it
// is not registered as an application import capability until scope is settled.
func NewPublicDormitoryHygieneXLSXParser(maxBytes int64) *XLSXParser {
	config := NewAttendanceXLSXParser(maxBytes).config
	config.ExpectedTemplateCode = "PUBLIC_DORMITORY_HYGIENE_IMPORT"
	config.ExpectedSheetName = "公共宿舍卫生导入"
	config.Fields = append([]TemplateField(nil), publicDormitoryHygieneImportFields...)
	return NewXLSXParser(config)
}
