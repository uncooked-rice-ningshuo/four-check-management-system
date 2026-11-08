package reportoutput

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"strings"

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
		{"DORMITORY_HYGIENE", "宿舍卫生检查情况", []column{date, field("寝室", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("情况", func(r reporting.Record) string { return r.Details.Result }), field("责任班级", func(r reporting.Record) string { return references(r.ResponsibleClasses) }), field("原因", func(r reporting.Record) string { return r.Details.Description }), remark}},
		{"PUBLIC_DORMITORY_HYGIENE", "宿舍公共卫生检查情况", []column{date, field("责任寝室", func(r reporting.Record) string { return references(r.ResponsibleDormitories) }), field("情况", func(r reporting.Record) string { return r.Details.Result }), field("责任班级", func(r reporting.Record) string { return references(r.ResponsibleClasses) }), field("原因", func(r reporting.Record) string { return r.Details.Description }), remark}},
		{"LATE_RETURN", "晚归检查情况", append(append([]column{}, common...), date, field("寝室", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("时间", func(r reporting.Record) string { return r.Details.Time }), remark)},
		{"CLASSROOM_HYGIENE", "教室卫生检查情况", []column{date, field("年级", func(r reporting.Record) string { return r.Grade }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("负责学生", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), field("情况", func(r reporting.Record) string { return r.Details.Description }), remark}},
	}
}

// wordRecordColumns follows the historical attachment tables where the
// immutable report snapshot has corresponding values. Legacy snapshots without
// adviser names keep those cells empty rather than reading mutable live data.
func wordRecordColumns(item reportType, teachers map[string]string, checkedDormitoryCount int) []column {
	field := func(title string, value func(reporting.Record) string) column { return column{title, value} }
	teacher := field("班主任", func(r reporting.Record) string { return teachers[classKey(r.Grade, r.ClassName)] })
	switch item.code {
	case "CLASSROOM_HYGIENE":
		return []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("日期", func(r reporting.Record) string { return r.Details.Date }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("负责人员", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), teacher, field("教室", func(r reporting.Record) string { return r.CurrentClassroomName }), field("违纪情况", func(r reporting.Record) string { return r.Details.Description })}
	case "DORMITORY_HYGIENE":
		return []column{field("日期", func(r reporting.Record) string { return r.Details.Date }), field("寝室号", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("情况", func(r reporting.Record) string { return r.Details.Description }), field("责任人", func(r reporting.Record) string { return references(r.ResponsibleStudents) }), field("所在班级", func(r reporting.Record) string { return r.ClassName }), teacher}
	case "PUBLIC_DORMITORY_HYGIENE":
		return []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("日期", func(r reporting.Record) string { return r.Details.Date }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("寝室号", func(r reporting.Record) string { return references(r.ResponsibleDormitories) }), teacher, field("负责宿舍楼", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("楼层/单元", func(r reporting.Record) string { return r.CurrentAreaName })}
	case "LATE_RETURN":
		return []column{field("年级", func(r reporting.Record) string { return r.Grade }), field("日期", func(r reporting.Record) string { return r.Details.Date }), field("班级", func(r reporting.Record) string { return r.ClassName }), field("寝室号", func(r reporting.Record) string { return r.CurrentDormitoryName }), field("晚归人员", func(r reporting.Record) string { return r.StudentName }), teacher, field("共查寝间数", func(reporting.Record) string { return fmt.Sprint(checkedDormitoryCount) }), {title: "缺勤率"}, field("备注", func(r reporting.Record) string { return r.Details.Remark })}
	default:
		return item.columns
	}
}

func classKey(grade, className string) string { return grade + "\x00" + className }

func headTeacherNames(content reporting.ReportVersionContent) map[string]string {
	result := make(map[string]string)
	if content.Indicators == nil {
		return result
	}
	for _, class := range content.Indicators.Inputs.Classes {
		result[classKey(class.Grade, class.ClassName)] = class.HeadTeacherName
	}
	return result
}

func checkedDormitoryCount(records []reporting.Record) int {
	seen := make(map[string]struct{})
	for _, record := range records {
		if name := strings.TrimSpace(record.CurrentDormitoryName); name != "" {
			seen[name] = struct{}{}
		}
		for _, dormitory := range record.ResponsibleDormitories {
			if name := strings.TrimSpace(dormitory.Label); name != "" {
				seen[name] = struct{}{}
			}
		}
	}
	return len(seen)
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

func classRecordValues(records []reporting.Record, code, grade, className string, value func(reporting.Record) string) string {
	seen := make(map[string]bool)
	var values []string
	for _, record := range records {
		if record.TypeCode != code || record.Grade != grade || record.ClassName != className {
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

func classRecordNames(records []reporting.Record, code, grade, className string) string {
	return classRecordValues(records, code, grade, className, func(record reporting.Record) string { return record.StudentName })
}

func classRecordCourses(records []reporting.Record, code, grade, className string) string {
	return classRecordValues(records, code, grade, className, func(record reporting.Record) string { return record.CurrentCourseName })
}

func classRecordAbsenceHours(records []reporting.Record, code, grade, className string) string {
	var total float64
	for _, record := range records {
		if record.TypeCode == code && record.Grade == grade && record.ClassName == className && record.Details.AbsenceHours != nil {
			total += *record.Details.AbsenceHours
		}
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

// outputTypeLayout exposes the same fixed attachment title and columns used
// by the DOCX renderer, so the settings page never invents a separate layout.
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
		headings := make([]string, 0, len(item.columns))
		for _, column := range wordRecordColumns(item, nil, 0) {
			headings = append(headings, column.title)
		}
		return item.title, headings
	}
	return "", nil
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
	property := ""
	if merge != "" {
		property = `<w:vMerge w:val="` + merge + `"/>`
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
	minimumHeight := 420
	if header {
		minimumHeight = 520
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
			title = strings.ReplaceAll(strings.ReplaceAll(title, "{semesterName}", content.Period.SemesterName), "{weekNo}", fmt.Sprint(content.Period.WeekNo))
			document.WriteString(coverTitle(title))
		}
		for index := range cover.Content {
			for run := range cover.Content[index].Content {
				value := cover.Content[index].Content[run].Text
				cover.Content[index].Content[run].Text = strings.ReplaceAll(strings.ReplaceAll(value, "{semesterName}", content.Period.SemesterName), "{weekNo}", fmt.Sprint(content.Period.WeekNo))
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
		document.WriteString(`<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId3"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:bottom="1440" w:left="1080" w:right="1080"/></w:sectPr></w:pPr></w:p>`)
	}
	teachers := headTeacherNames(content)
	dormitoryCount := checkedDormitoryCount(records)
	for typeIndex, item := range types {
		if typeIndex > 0 {
			document.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		document.WriteString(attachmentTitle(item.title, item.code))
		columns := wordRecordColumns(item, teachers, dormitoryCount)
		var lines [][]string
		if content.Indicators != nil && !outputSourceUnsubmitted(content, item.code) && (item.code == "ATTENDANCE_RATE" || item.code == "MORNING_ATTENDANCE") {
			if item.code == "ATTENDANCE_RATE" {
				columns = []column{{title: "年级"}, {title: "班级"}, {title: "应到/实到"}, {title: "缺课同学"}, {title: "课程"}, {title: "班主任"}, {title: "缺课总课时"}, {title: "到课率"}}
			} else {
				columns = []column{{title: "年级"}, {title: "班级"}, {title: "应到/实到"}, {title: "缺到人员"}, {title: "班主任"}, {title: "缺勤总次数"}, {title: "出勤率"}}
			}
			for _, class := range content.Indicators.Classes {
				values := []string{class.Grade, class.ClassName}
				if item.code == "ATTENDANCE_RATE" && class.AttendanceRate != nil {
					m := class.AttendanceRate
					values = append(values, fmt.Sprintf("%d/%d", m.ExpectedStudentCount, m.AttendedStudentCount), classRecordNames(records, item.code, class.Grade, class.ClassName), classRecordCourses(records, item.code, class.Grade, class.ClassName), teachers[classKey(class.Grade, class.ClassName)], classRecordAbsenceHours(records, item.code, class.Grade, class.ClassName), rate(m.Rate))
				}
				if item.code == "MORNING_ATTENDANCE" && class.MorningAttendance != nil {
					m := class.MorningAttendance
					values = append(values, fmt.Sprintf("%d/%d", m.ExpectedPersonTimes, m.AttendedPersonTimes), classRecordNames(records, item.code, class.Grade, class.ClassName), teachers[classKey(class.Grade, class.ClassName)], fmt.Sprint(m.AbsencePersonTimes), rate(m.Rate))
				}
				if len(values) == len(columns) {
					lines = append(lines, values)
				}
			}
		} else {
			for _, record := range records {
				if record.TypeCode != item.code {
					continue
				}
				values := make([]string, 0, len(columns))
				for _, definition := range columns {
					if definition.value == nil {
						values = append(values, "")
						continue
					}
					values = append(values, definition.value(record))
				}
				lines = append(lines, values)
			}
		}
		if len(lines) == 0 {
			empty := make([]string, len(columns))
			empty[0] = emptyOutputText(content, item.code)
			lines = append(lines, empty)
		}
		widths := wordColumnWidths(columns)
		pages := paginateWordRows(lines, widths)
		for pageIndex, page := range pages {
			if pageIndex > 0 {
				document.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
				document.WriteString(attachmentTitle(item.title+"（续）", item.code))
			}
			var grid strings.Builder
			for _, width := range widths {
				grid.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, width))
			}
			document.WriteString(fmt.Sprintf(`<w:tbl><w:tblPr><w:tblW w:w="15000" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders><w:top w:val="single"/><w:left w:val="single"/><w:bottom w:val="single"/><w:right w:val="single"/><w:insideH w:val="single"/><w:insideV w:val="single"/></w:tblBorders></w:tblPr><w:tblGrid>%s</w:tblGrid>`, grid.String()))
			headings := make([]string, 0, len(columns))
			for _, c := range columns {
				headings = append(headings, c.title)
			}
			hygiene := item.code == "DORMITORY_HYGIENE" || item.code == "PUBLIC_DORMITORY_HYGIENE" || item.code == "CLASSROOM_HYGIENE"
			document.WriteString(row(headings, widths, "", true, hygiene))
			for index, line := range page {
				gradeMerge := ""
				if content.Indicators != nil && !outputSourceUnsubmitted(content, item.code) && (item.code == "ATTENDANCE_RATE" || item.code == "MORNING_ATTENDANCE") && line[0] != "本周无已确认违纪记录" {
					gradeMerge = "restart"
					if index > 0 && page[index-1][0] == line[0] {
						gradeMerge = "continue"
					}
				}
				document.WriteString(row(line, widths, gradeMerge, false, hygiene))
			}
			document.WriteString(`</w:tbl>`)
		}
	}
	document.WriteString(`<w:sectPr><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId3"/><w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/><w:pgMar w:top="900" w:bottom="900" w:left="900" w:right="900"/></w:sectPr></w:body></w:document>`)
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
