package reportoutput

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strings"

	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/reporting"
	"github.com/xuri/excelize/v2"
)

type column struct {
	title string
	value func(reporting.Record) string
}
type reportType struct {
	code, title string
	columns     []column
}

func recordColumns() []reportType {
	field := func(title string, value func(reporting.Record) string) column { return column{title, value} }
	common := []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("姓名", func(r reporting.Record) string { return r.StudentName }), field("学号", func(r reporting.Record) string { return r.StudentNumber })}
	date := field("日期", func(r reporting.Record) string { return r.Details.Date })
	remark := field("备注", func(r reporting.Record) string { return r.Details.Remark })
	return []reportType{
		{"ATTENDANCE_RATE", "到课率检查情况", append(append([]column{}, common...), date, field("课程", func(r reporting.Record) string { return r.CurrentCourseName }), field("本周旷课学时", func(r reporting.Record) string { return number(r.Details.AbsenceHours) }), remark)},
		{"UNATTENDED_CLASSROOM", "无机课堂检查情况", append(append([]column{}, common...), date, field("课程", func(r reporting.Record) string { return r.CurrentCourseName }), field("节次", func(r reporting.Record) string { return r.Details.Period }), field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), remark)},
		{"MORNING_ATTENDANCE", "早锻炼早签检查情况", append(append([]column{}, common...), date, field("缺勤次数", func(r reporting.Record) string { return integer(r.Details.AbsenceCount) }), remark)},
		{"EVENING_SELF_STUDY", "晚自习检查情况", append(append([]column{}, common...), date, field("节次", func(r reporting.Record) string { return r.Details.Period }), field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), remark)},
		{"DORMITORY_HYGIENE", "宿舍卫生检查情况", []column{date, field("寝室", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("情况", func(r reporting.Record) string { return hygieneSituation(r.Details) }), field("责任班级", func(r reporting.Record) string { return references(r.ResponsibleClasses) }), remark}},
		{"PUBLIC_DORMITORY_HYGIENE", "宿舍公共卫生检查情况", []column{date, field("责任寝室", func(r reporting.Record) string { return references(r.ResponsibleDormitories) }), field("情况", func(r reporting.Record) string { return hygieneSituation(r.Details) }), field("责任班级", func(r reporting.Record) string { return references(r.ResponsibleClasses) }), remark}},
		{"LATE_RETURN", "晚归检查情况", append(append([]column{}, common...), date, field("寝室", func(r reporting.Record) string { return r.CurrentDormitoryName }), remark)},
		{"CLASSROOM_HYGIENE", "教室卫生检查情况", []column{date, field("年级", func(r reporting.Record) string { return r.Grade }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("负责学生", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), field("情况", func(r reporting.Record) string { return hygieneSituation(r.Details) }), remark}},
	}
}

// wordRecordColumns follows the historical attachment tables where the
// immutable report snapshot has corresponding values. Legacy snapshots without
// adviser names keep those cells empty rather than reading mutable live data.
func wordRecordColumns(item reportType, teachers map[string]string) []column {
	field := func(title string, value func(reporting.Record) string) column { return column{title, value} }
	teacher := field("班主任", func(r reporting.Record) string { return teachers[classOutputKey(r.ClassID, r.Grade, r.ClassName)] })
	switch item.code {
	case "CLASSROOM_HYGIENE":
		return []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("日期", func(r reporting.Record) string { return r.Details.Date }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("负责人员", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), teacher, field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), field("检查情况", func(r reporting.Record) string { return hygieneSituation(r.Details) })}
	case "DORMITORY_HYGIENE":
		return []column{field("日期", func(r reporting.Record) string { return r.Details.Date }), field("寝室号", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("情况", func(r reporting.Record) string { return hygieneSituation(r.Details) }), field("责任人", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), field("所在班级", responsibleClassNames), teacher}
	case "PUBLIC_DORMITORY_HYGIENE":
		return []column{field("日期", func(r reporting.Record) string { return r.Details.Date }), field("责任寝室", func(r reporting.Record) string { return references(r.ResponsibleDormitories) }), field("情况", func(r reporting.Record) string { return hygieneSituation(r.Details) }), field("责任班级", func(r reporting.Record) string { return references(r.ResponsibleClasses) }), field("备注", func(r reporting.Record) string { return r.Details.Remark })}
	case "LATE_RETURN":
		return []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("日期", func(r reporting.Record) string { return r.Details.Date }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("寝室号", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("晚归人员", func(r reporting.Record) string { return r.StudentName }), teacher, field("备注", func(r reporting.Record) string { return r.Details.Remark })}
	default:
		return item.columns
	}
}

func classKey(grade, className string) string { return grade + "\x00" + className }
func classOutputKey(classID, grade, className string) string {
	if classID != "" {
		return "id:" + classID
	}
	return "legacy:" + classKey(grade, className)
}

func headTeacherNames(content reporting.ReportVersionContent) map[string]string {
	result := make(map[string]string)
	if content.Indicators == nil {
		return result
	}
	for _, class := range content.Indicators.Inputs.Classes {
		result[classOutputKey(class.ClassID, class.Grade, class.ClassName)] = class.HeadTeacherName
	}
	return result
}

func hygieneSituation(details reporting.RecordDetails) string {
	result := map[string]string{inspection.HygieneExcellent: "优秀", inspection.HygieneQualified: "合格", inspection.HygieneUnqualified: "不合格"}[details.Result]
	if result == "" {
		return details.Description
	}
	if details.Description == "" {
		return result
	}
	return result + "：" + details.Description
}

func number(value *float64) string {
	if value == nil {
		return ""
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) {
		return ""
	}
	return fmt.Sprintf("%g", *value)
}
func integer(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}
func references(values []reporting.RecordReference) string {
	names := make([]string, 0, len(values))
	for _, v := range values {
		if v.Label != "" {
			names = append(names, v.Label)
		}
	}
	return strings.Join(names, "、")
}

func responsibleClassNames(record reporting.Record) string {
	if names := references(record.ResponsibleClasses); names != "" {
		return names
	}
	return record.ClassName
}

func recordInReportClass(record reporting.Record, class reporting.ReportClassIndicators) bool {
	if class.ClassID != "" && record.ClassID != "" {
		return class.ClassID == record.ClassID
	}
	return record.Grade == class.Grade && record.ClassName == class.ClassName
}

func classRecordValues(records []reporting.Record, code string, class reporting.ReportClassIndicators, value func(reporting.Record) string) string {
	seen := make(map[string]bool)
	var values []string
	for _, record := range records {
		if record.TypeCode != code || !recordInReportClass(record, class) {
			continue
		}
		item := strings.TrimSpace(value(record))
		if item != "" && !seen[item] {
			seen[item] = true
			values = append(values, item)
		}
	}
	return strings.Join(values, "、")
}

func classRecordNames(records []reporting.Record, code string, class reporting.ReportClassIndicators) string {
	seen := make(map[string]bool)
	var names []string
	for _, record := range records {
		if record.TypeCode != code || !recordInReportClass(record, class) {
			continue
		}
		name := strings.TrimSpace(record.StudentName)
		if name == "" {
			continue
		}
		// A student can have several absence records; names alone cannot identify
		// distinct students when two classmates share the same name.
		identity := strings.TrimSpace(record.StudentNumber)
		if identity == "" {
			identity = "name:" + name
		} else {
			identity = "number:" + identity
		}
		if !seen[identity] {
			seen[identity] = true
			names = append(names, name)
		}
	}
	return strings.Join(names, "、")
}

func attendanceWordLines(content reporting.ReportVersionContent, records []reporting.Record, teachers map[string]string) [][]string {
	classes := reportWordClasses(content, records, "ATTENDANCE_RATE")
	metricsAvailable := content.Indicators != nil && !outputSourceUnsubmitted(content, "ATTENDANCE_RATE")
	lines := make([][]string, 0, len(classes))
	for _, class := range classes {
		key := classOutputKey(class.ClassID, class.Grade, class.ClassName)
		values := []string{class.Grade, wordClassLabel(class, classes), "", classRecordNames(records, "ATTENDANCE_RATE", class), classRecordCourses(records, "ATTENDANCE_RATE", class), teachers[key], classRecordAbsenceHours(records, "ATTENDANCE_RATE", class), ""}
		if metricsAvailable && class.AttendanceRate != nil {
			metric := class.AttendanceRate
			values[2] = fmt.Sprintf("%d/%d", metric.ExpectedStudentCount, metric.AttendedStudentCount)
			values[7] = rate(metric.Rate)
		}
		lines = append(lines, values)
	}
	return lines
}

func reportWordClasses(content reporting.ReportVersionContent, records []reporting.Record, code string) []reporting.ReportClassIndicators {
	classes := make(map[string]reporting.ReportClassIndicators)
	metricsAvailable := content.Indicators != nil && !outputSourceUnsubmitted(content, code)
	if metricsAvailable {
		for _, class := range content.Indicators.Classes {
			classes[classOutputKey(class.ClassID, class.Grade, class.ClassName)] = class
		}
	}
	for _, record := range records {
		if record.TypeCode == code && record.Grade != "" && record.ClassName != "" {
			key := classOutputKey(record.ClassID, record.Grade, record.ClassName)
			if _, exists := classes[key]; !exists {
				classes[key] = reporting.ReportClassIndicators{ClassID: record.ClassID, Grade: record.Grade, ClassName: record.ClassName}
			}
		}
	}
	keys := make([]string, 0, len(classes))
	for key := range classes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := classes[keys[i]], classes[keys[j]]
		if left.Grade != right.Grade {
			return left.Grade < right.Grade
		}
		if left.ClassName != right.ClassName {
			return left.ClassName < right.ClassName
		}
		return keys[i] < keys[j]
	})
	result := make([]reporting.ReportClassIndicators, 0, len(keys))
	for _, key := range keys {
		result = append(result, classes[key])
	}
	return result
}

func wordClassLabel(class reporting.ReportClassIndicators, classes []reporting.ReportClassIndicators) string {
	for _, other := range classes {
		if other.ClassID != class.ClassID && other.Grade == class.Grade && other.ClassName == class.ClassName {
			parts := []string{}
			if class.FacultyName != "" {
				parts = append(parts, class.FacultyName)
			}
			if class.MajorName != "" {
				parts = append(parts, class.MajorName)
			}
			if len(parts) > 0 {
				return class.ClassName + "（" + strings.Join(parts, "·") + "）"
			}
		}
	}
	return class.ClassName
}

func classRecordCount(records []reporting.Record, code string, class reporting.ReportClassIndicators) int {
	count := 0
	for _, record := range records {
		if record.TypeCode == code && recordInReportClass(record, class) {
			count++
		}
	}
	return count
}

func morningWordLines(content reporting.ReportVersionContent, records []reporting.Record, teachers map[string]string) [][]string {
	classes := reportWordClasses(content, records, "MORNING_ATTENDANCE")
	metricsAvailable := content.Indicators != nil && !outputSourceUnsubmitted(content, "MORNING_ATTENDANCE")
	lines := make([][]string, 0, len(classes))
	for _, class := range classes {
		values := []string{class.Grade, wordClassLabel(class, classes), "", classRecordNames(records, "MORNING_ATTENDANCE", class), teachers[classOutputKey(class.ClassID, class.Grade, class.ClassName)], "", ""}
		if metricsAvailable && class.MorningAttendance != nil {
			metric := class.MorningAttendance
			values[2] = fmt.Sprintf("%d/%d", metric.ExpectedPersonTimes, metric.AttendedPersonTimes)
			values[5] = fmt.Sprint(metric.AbsencePersonTimes)
			values[6] = rate(metric.Rate)
		}
		lines = append(lines, values)
	}
	return lines
}

func unattendedWordLines(content reporting.ReportVersionContent, records []reporting.Record, teachers map[string]string) [][]string {
	classes := reportWordClasses(content, records, "UNATTENDED_CLASSROOM")
	metricsAvailable := content.Indicators != nil && !outputSourceUnsubmitted(content, "UNATTENDED_CLASSROOM")
	lines := make([][]string, 0, len(classes))
	for _, class := range classes {
		count := classRecordCount(records, "UNATTENDED_CLASSROOM", class)
		if metricsAvailable && class.UnattendedClassroom != nil {
			count = class.UnattendedClassroom.RawRecordCount
		}
		lines = append(lines, []string{class.Grade, wordClassLabel(class, classes), classRecordNames(records, "UNATTENDED_CLASSROOM", class), classRecordCourses(records, "UNATTENDED_CLASSROOM", class), teachers[classOutputKey(class.ClassID, class.Grade, class.ClassName)], fmt.Sprint(count)})
	}
	return lines
}

func eveningWordLines(content reporting.ReportVersionContent, records []reporting.Record, teachers map[string]string) [][]string {
	classes := reportWordClasses(content, records, "EVENING_SELF_STUDY")
	metricsAvailable := content.Indicators != nil && !outputSourceUnsubmitted(content, "EVENING_SELF_STUDY")
	lines := make([][]string, 0, len(classes))
	for _, class := range classes {
		count := classRecordCount(records, "EVENING_SELF_STUDY", class)
		if metricsAvailable && class.EveningSelfStudy != nil {
			count = class.EveningSelfStudy.RawRecordCount
		}
		lines = append(lines, []string{class.Grade, wordClassLabel(class, classes), classRecordNames(records, "EVENING_SELF_STUDY", class), teachers[classOutputKey(class.ClassID, class.Grade, class.ClassName)], fmt.Sprint(count)})
	}
	return lines
}

func classRecordCourses(records []reporting.Record, code string, class reporting.ReportClassIndicators) string {
	return classRecordValues(records, code, class, func(record reporting.Record) string { return record.CurrentCourseName })
}

func classRecordAbsenceHours(records []reporting.Record, code string, class reporting.ReportClassIndicators) string {
	var total float64
	found := false
	for _, record := range records {
		if record.TypeCode == code && recordInReportClass(record, class) && record.Details.AbsenceHours != nil {
			total += *record.Details.AbsenceHours
			found = true
		}
	}
	if !found {
		return ""
	}
	return fmt.Sprintf("%g", total)
}
func safeCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@", rune(value[0])) {
		return "'" + value
	}
	return value
}

func chosenTypes(codes []string) ([]reportType, error) {
	catalog := recordColumns()
	lookup := make(map[string]reportType, len(catalog))
	for _, item := range catalog {
		lookup[item.code] = item
	}
	if len(codes) == 0 || len(codes) > len(catalog) {
		return nil, ErrInvalidSelection
	}
	seen := make(map[string]bool, len(codes))
	result := make([]reportType, 0, len(codes))
	for _, code := range codes {
		item, ok := lookup[code]
		if !ok || seen[code] {
			return nil, ErrInvalidSelection
		}
		seen[code] = true
		result = append(result, item)
	}
	return result, nil
}

// outputTypeLayout exposes the attachment title and primary columns used by
// the DOCX renderer; dormitory subtable headings are in wordOutputSections.
func outputTypeLayout(code string) (string, []string) {
	for _, item := range recordColumns() {
		if item.code != code {
			continue
		}
		if code == "ATTENDANCE_RATE" {
			return item.title, []string{"年级", "班级", "应到/实到", "缺课同学", "课程", "班主任", "缺课总课时", "到课率"}
		}
		if code == "MORNING_ATTENDANCE" {
			return item.title, []string{"年级", "班级", "应到/实到", "缺到人员", "班主任", "缺勤总次数", "出勤率"}
		}
		if code == "UNATTENDED_CLASSROOM" {
			return item.title, []string{"年级", "班级", "违纪学生", "课程", "班主任", "违规次数"}
		}
		if code == "EVENING_SELF_STUDY" {
			return item.title, []string{"年级", "班级", "缺勤学生", "班主任", "缺勤次数"}
		}
		headings := make([]string, 0, len(item.columns))
		for _, column := range wordRecordColumns(item, nil) {
			headings = append(headings, column.title)
		}
		return item.title, headings
	}
	return "", nil
}

func wordOutputSections(code string, columns []string) []OutputSection {
	if code != "DORMITORY_HYGIENE" {
		return nil
	}
	excellent := []string{"寝室号", "所在班级", "责任人", "班主任"}
	return []OutputSection{
		{Title: "不合格寝室", Columns: columns},
		{Title: "优秀寝室", Columns: append(append([]string{}, excellent...), excellent...)},
		{Title: "合格寝室", Columns: excellent},
		{Title: "其他宿舍检查记录", Columns: columns},
	}
}

func excelTypeColumns(code string) []string {
	for _, item := range recordColumns() {
		if item.code == code {
			columns := make([]string, 0, len(item.columns))
			for _, column := range item.columns {
				columns = append(columns, column.title)
			}
			return columns
		}
	}
	return nil
}

// The frozen version distinguishes an unsubmitted source from a confirmed empty one.
func outputSourceUnsubmitted(content reporting.ReportVersionContent, code string) bool {
	for _, row := range content.Rows {
		if row.InspectionTypeCode == code && row.DataStatus == "NO_CONFIRMED_DATA" {
			return true
		}
	}
	return false
}

func emptyOutputText(content reporting.ReportVersionContent, code string) string {
	if outputSourceUnsubmitted(content, code) {
		return "来源未完成，暂无已确认数据"
	}
	return "本周无已确认违纪记录"
}

// RenderExcel creates only the selected inspection sheets from immutable records.
func RenderExcel(content reporting.ReportVersionContent, records []reporting.Record, codes []string) ([]byte, error) {
	types, err := chosenTypes(codes)
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	first := book.GetSheetName(0)
	titleStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "宋体", Size: 18, Bold: true}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	if err != nil {
		return nil, err
	}
	headerStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "宋体", Size: 12, Bold: true}, Border: []excelize.Border{{Type: "bottom", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "left", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, err
	}
	bodyStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "宋体", Size: 12}, NumFmt: 49, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, err
	}
	morningBodyStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "宋体", Size: 11}, NumFmt: 49, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, err
	}
	for index, item := range types {
		name := item.title
		if index == 0 {
			if err := book.SetSheetName(first, name); err != nil {
				return nil, err
			}
		} else {
			if _, err := book.NewSheet(name); err != nil {
				return nil, err
			}
		}
		last, err := excelize.ColumnNumberToName(len(item.columns))
		if err != nil {
			return nil, err
		}
		if err := book.MergeCell(name, "A1", last+"1"); err != nil {
			return nil, err
		}
		if err := book.SetCellStr(name, "A1", fmt.Sprintf("%s 第%d周 %s", content.Period.SemesterName, content.Period.WeekNo, item.title)); err != nil {
			return nil, err
		}
		if err := book.SetCellStyle(name, "A1", last+"1", titleStyle); err != nil {
			return nil, err
		}
		if err := book.SetRowHeight(name, 1, 30); err != nil {
			return nil, err
		}
		for column, definition := range item.columns {
			letter, _ := excelize.ColumnNumberToName(column + 1)
			if err := book.SetCellStr(name, letter+"2", definition.title); err != nil {
				return nil, err
			}
			if err := book.SetColWidth(name, letter, letter, 22); err != nil {
				return nil, err
			}
		}
		if err := book.SetCellStyle(name, "A2", last+"2", headerStyle); err != nil {
			return nil, err
		}
		rowNo := 3
		for _, record := range records {
			if record.TypeCode != item.code {
				continue
			}
			for column, definition := range item.columns {
				cell, _ := excelize.CoordinatesToCellName(column+1, rowNo)
				if err := book.SetCellStr(name, cell, safeCell(definition.value(record))); err != nil {
					return nil, err
				}
			}
			rowNo++
		}
		if rowNo == 3 {
			if err := book.SetCellStr(name, "A3", emptyOutputText(content, item.code)); err != nil {
				return nil, err
			}
			rowNo++
		}
		itemBodyStyle := bodyStyle
		if item.code == "MORNING_ATTENDANCE" {
			itemBodyStyle = morningBodyStyle
		}
		if err := book.SetCellStyle(name, "A3", fmt.Sprintf("%s%d", last, rowNo-1), itemBodyStyle); err != nil {
			return nil, err
		}
		if err := book.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft"}); err != nil {
			return nil, err
		}
	}
	book.SetActiveSheet(0)
	var buffer bytes.Buffer
	if err := book.Write(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func xmlText(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
func paragraph(value string) string {
	return `<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="SimSun" w:eastAsia="宋体"/><w:sz w:val="28"/></w:rPr><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p>`
}
func cell(value string, width int, merge string, header bool, hygiene bool) string {
	property := `<w:vAlign w:val="center"/>`
	if merge != "" {
		property += `<w:vMerge w:val="` + merge + `"/>`
	}
	font := "仿宋"
	if hygiene {
		font = "仿宋_GB2312"
	}
	size, bold := "24", ""
	if header {
		size, bold = "30", `<w:b/>`
	}
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="FangSong" w:eastAsia="%s"/><w:sz w:val="%s"/>%s</w:rPr><w:t xml:space="preserve">`, width, property, font, size, bold) + xmlText(value) + `</w:t></w:r></w:p></w:tc>`
}
func row(values []string, widths []int, gradeMerge string, header bool, hygiene bool) string {
	var b strings.Builder
	minimumHeight := 600
	if header {
		minimumHeight = 600
	}
	b.WriteString(fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="atLeast"/>`, minimumHeight))
	if header {
		b.WriteString(`<w:tblHeader/>`)
	}
	if estimatedRowHeight(values, widths) <= 6500 {
		b.WriteString(`<w:cantSplit/>`)
	}
	b.WriteString(`</w:trPr>`)
	for index, value := range values {
		merge := ""
		if index == 0 {
			merge = gradeMerge
			if gradeMerge == "continue" {
				value = ""
			}
		}
		b.WriteString(cell(value, widths[index], merge, header, hygiene))
	}
	b.WriteString(`</w:tr>`)
	return b.String()
}

func sortedWordRecords(records []reporting.Record, code string) []reporting.Record {
	selected := make([]reporting.Record, 0)
	for _, record := range records {
		if record.TypeCode == code {
			selected = append(selected, record)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		for index, value := range []string{left.Grade, left.ClassName, left.Details.Date, left.CurrentDormitoryName, left.StudentNumber, left.ID} {
			other := []string{right.Grade, right.ClassName, right.Details.Date, right.CurrentDormitoryName, right.StudentNumber, right.ID}[index]
			if value != other {
				return value < other
			}
		}
		return false
	})
	return selected
}

func wordRecordLines(records []reporting.Record, columns []column) [][]string {
	lines := make([][]string, 0, len(records))
	for _, record := range records {
		values := make([]string, 0, len(columns))
		for _, definition := range columns {
			if definition.value == nil {
				values = append(values, "")
			} else {
				values = append(values, definition.value(record))
			}
		}
		lines = append(lines, values)
	}
	return lines
}

func renderWordTablePages(document *strings.Builder, title, code string, columns []column, lines [][]string, grouped bool) {
	widths := wordColumnWidths(columns)
	pages := paginateWordRows(lines, widths)
	for pageIndex, page := range pages {
		if pageIndex > 0 {
			document.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
			document.WriteString(attachmentTitle(title+"（续）", code))
		}
		var grid strings.Builder
		for _, width := range widths {
			grid.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, width))
		}
		document.WriteString(fmt.Sprintf(`<w:tbl><w:tblPr><w:tblW w:w="15000" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders><w:top w:val="single"/><w:left w:val="single"/><w:bottom w:val="single"/><w:right w:val="single"/><w:insideH w:val="single"/><w:insideV w:val="single"/></w:tblBorders></w:tblPr><w:tblGrid>%s</w:tblGrid>`, grid.String()))
		headings := make([]string, 0, len(columns))
		for _, item := range columns {
			headings = append(headings, item.title)
		}
		hygiene := code == "DORMITORY_HYGIENE" || code == "PUBLIC_DORMITORY_HYGIENE" || code == "CLASSROOM_HYGIENE"
		document.WriteString(row(headings, widths, "", true, hygiene))
		for index, line := range page {
			values := append([]string(nil), line...)
			if len(values) > 0 && !strings.HasPrefix(values[0], "本周无") && !strings.HasPrefix(values[0], "来源未完成") {
				for columnIndex, definition := range columns {
					if values[columnIndex] == "" && (definition.title == "缺课同学" || definition.title == "课程" || definition.title == "缺到人员" || definition.title == "违纪学生" || definition.title == "缺勤学生") {
						values[columnIndex] = "无"
					}
				}
			}
			gradeMerge := ""
			if grouped && values[1] != "" {
				gradeMerge = "restart"
				if index > 0 && page[index-1][0] == line[0] {
					gradeMerge = "continue"
				}
			}
			document.WriteString(row(values, widths, gradeMerge, false, hygiene))
		}
		document.WriteString(`</w:tbl>`)
	}
}

func dormitoryWordTables(document *strings.Builder, content reporting.ReportVersionContent, records []reporting.Record, item reportType, teachers map[string]string) {
	field := func(title string, value func(reporting.Record) string) column { return column{title, value} }
	excellentColumns := []column{
		field("寝室号", func(r reporting.Record) string { return r.CurrentDormitoryName }),
		field("所在班级", responsibleClassNames),
		field("责任人", func(r reporting.Record) string { return references(r.ResponsibleStudents) }),
		field("班主任", func(r reporting.Record) string { return teachers[classOutputKey(r.ClassID, r.Grade, r.ClassName)] }),
	}
	pairedExcellentColumns := append(append([]column{}, excellentColumns...), excellentColumns...)
	groups := []struct {
		result, title string
		columns       []column
	}{
		{inspection.HygieneUnqualified, "不合格寝室", wordRecordColumns(item, teachers)},
		{inspection.HygieneExcellent, "优秀寝室", pairedExcellentColumns},
		{inspection.HygieneQualified, "合格寝室", excellentColumns},
	}
	selected := sortedWordRecords(records, item.code)
	written := false
	for _, group := range groups {
		var members []reporting.Record
		for _, record := range selected {
			if record.Details.Result == group.result {
				members = append(members, record)
			}
		}
		if len(members) == 0 {
			continue
		}
		document.WriteString(attachmentTitle(group.title, item.code))
		lines := wordRecordLines(members, group.columns)
		if group.result == inspection.HygieneExcellent {
			lines = nil
			for index := 0; index < len(members); index += 2 {
				line := make([]string, len(pairedExcellentColumns))
				for offset := 0; offset < 2 && index+offset < len(members); offset++ {
					for columnIndex, column := range excellentColumns {
						line[offset*len(excellentColumns)+columnIndex] = column.value(members[index+offset])
					}
				}
				lines = append(lines, line)
			}
		}
		renderWordTablePages(document, group.title, item.code, group.columns, lines, false)
		written = true
	}
	var other []reporting.Record
	for _, record := range selected {
		if record.Details.Result != inspection.HygieneUnqualified && record.Details.Result != inspection.HygieneExcellent && record.Details.Result != inspection.HygieneQualified {
			other = append(other, record)
		}
	}
	if len(other) > 0 {
		columns := wordRecordColumns(item, teachers)
		document.WriteString(attachmentTitle("其他宿舍检查记录", item.code))
		renderWordTablePages(document, "其他宿舍检查记录", item.code, columns, wordRecordLines(other, columns), false)
		written = true
	}
	if !written {
		columns := wordRecordColumns(item, teachers)
		empty := make([]string, len(columns))
		empty[0] = emptyOutputText(content, item.code)
		renderWordTablePages(document, item.title, item.code, columns, [][]string{empty}, false)
	}
}

// RenderWord builds a standard DOCX with a portrait introduction and landscape inspection pages.
func RenderWord(content reporting.ReportVersionContent, records []reporting.Record, codes []string, introduction string, combined bool) ([]byte, error) {
	return RenderWordDocument(content, records, codes, legacyIntroduction(introduction), combined)
}

// RenderWordDocument maps only validated cover formatting to OOXML. The report
// snapshot remains authoritative; this function does not mutate report data.
func RenderWordDocument(content reporting.ReportVersionContent, records []reporting.Record, codes []string, introduction json.RawMessage, combined bool) ([]byte, error) {
	types, err := chosenTypes(codes)
	if err != nil {
		return nil, err
	}
	var cover IntroductionDocument
	if combined {
		cover, err = decodeIntroduction(introduction)
		if err != nil {
			return nil, err
		}
	}
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	if combined {
		hasStyledTitle := false
		for _, paragraph := range cover.Content {
			if paragraph.Attrs.Style == "title" || paragraph.Attrs.Style == "subtitle" {
				hasStyledTitle = true
				break
			}
		}
		if !hasStyledTitle {
			title := cover.TitleTemplate
			if title == "" {
				title = "{semesterName} 第{weekNo}周日常管理通报"
			}
			title = strings.NewReplacer("{semesterName}", content.Period.SemesterName, "{weekNoChinese}", chineseNumber(content.Period.WeekNo), "{weekNo}", fmt.Sprint(content.Period.WeekNo)).Replace(title)
			document.WriteString(coverTitle(title))
		}
		for index := range cover.Content {
			for run := range cover.Content[index].Content {
				value := cover.Content[index].Content[run].Text
				cover.Content[index].Content[run].Text = strings.NewReplacer("{semesterName}", content.Period.SemesterName, "{weekNoChinese}", chineseNumber(content.Period.WeekNo), "{weekNo}", fmt.Sprint(content.Period.WeekNo)).Replace(value)
			}
		}
		position := len(cover.Content)
		if cover.AttachmentPosition != nil {
			position = *cover.AttachmentPosition
		}
		document.WriteString(renderIntroduction(IntroductionDocument{Content: cover.Content[:position]}))
		if !cover.ManualAttachmentList {
			document.WriteString(coverAttachmentList(types))
		}
		document.WriteString(renderIntroduction(IntroductionDocument{Content: cover.Content[position:]}))
		document.WriteString(`<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId3"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:bottom="1440" w:left="1080" w:right="1080"/>` + wordPageNumberFormat(cover) + `</w:sectPr></w:pPr></w:p>`)
	}
	teachers := headTeacherNames(content)
	for typeIndex, item := range types {
		if typeIndex > 0 {
			document.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		wordTitle := wordAttachmentTitle(item.code, content.Period.WeekNo)
		document.WriteString(attachmentTitle(wordTitle, item.code))
		if item.code == "DORMITORY_HYGIENE" {
			dormitoryWordTables(&document, content, records, item, teachers)
			continue
		}
		columns := wordRecordColumns(item, teachers)
		var lines [][]string
		grouped := false
		if item.code == "ATTENDANCE_RATE" {
			columns = []column{{title: "年级"}, {title: "班级"}, {title: "应到/实到"}, {title: "缺课同学"}, {title: "课程"}, {title: "班主任"}, {title: "缺课总课时"}, {title: "到课率"}}
			lines = attendanceWordLines(content, records, teachers)
			grouped = true
		} else if item.code == "MORNING_ATTENDANCE" {
			columns = []column{{title: "年级"}, {title: "班级"}, {title: "应到/实到"}, {title: "缺到人员"}, {title: "班主任"}, {title: "缺勤总次数"}, {title: "出勤率"}}
			lines = morningWordLines(content, records, teachers)
			grouped = true
		} else if item.code == "UNATTENDED_CLASSROOM" {
			columns = []column{{title: "年级"}, {title: "班级"}, {title: "违纪学生"}, {title: "课程"}, {title: "班主任"}, {title: "违规次数"}}
			lines = unattendedWordLines(content, records, teachers)
			grouped = true
		} else if item.code == "EVENING_SELF_STUDY" {
			columns = []column{{title: "年级"}, {title: "班级"}, {title: "缺勤学生"}, {title: "班主任"}, {title: "缺勤次数"}}
			lines = eveningWordLines(content, records, teachers)
			grouped = true
		} else {
			lines = wordRecordLines(sortedWordRecords(records, item.code), columns)
		}
		if len(lines) == 0 {
			empty := make([]string, len(columns))
			empty[0] = emptyOutputText(content, item.code)
			lines = append(lines, empty)
		}
		renderWordTablePages(&document, wordTitle, item.code, columns, lines, grouped)
	}
	document.WriteString(`<w:sectPr><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId3"/><w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/><w:pgMar w:top="900" w:bottom="900" w:left="900" w:right="900"/>` + wordPageNumberFormat(cover) + `</w:sectPr></w:body></w:document>`)
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := []struct{ name, value string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/><Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/><Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/></Relationships>`},
		{"word/styles.xml", wordStylesXML},
		{"word/settings.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:updateFields w:val="true"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`},
		{"word/header1.xml", wordHeaderXML(cover.HeaderText)},
		{"word/footer1.xml", wordFooterDocumentXML(cover, content.Period.SemesterName, content.Period.WeekNo)},
		{"word/document.xml", document.String()},
	}
	for _, file := range files {
		writer, err := archive.Create(file.name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(file.value)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func rate(value *float64) string {
	if value == nil {
		return "不可计算"
	}
	return fmt.Sprintf("%.2f%%", *value)
}
