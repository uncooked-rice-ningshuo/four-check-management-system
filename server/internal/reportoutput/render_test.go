package reportoutput

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"four-check-management-system/server/internal/reporting"
	"github.com/xuri/excelize/v2"
)

func TestRenderSelectedOutputsUseFrozenData(t *testing.T) {
	content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}, Indicators: &reporting.ReportIndicators{Classes: []reporting.ReportClassIndicators{{Grade: "2026级", ClassName: "一班", ReportIndicatorValues: reporting.ReportIndicatorValues{AttendanceRate: &reporting.ReportAttendanceRateMetric{ExpectedStudentCount: 2, AttendedStudentCount: 1, ViolationStudentCount: 1}}}, {Grade: "2026级", ClassName: "二班", ReportIndicatorValues: reporting.ReportIndicatorValues{AttendanceRate: &reporting.ReportAttendanceRateMetric{ExpectedStudentCount: 0, AttendedStudentCount: 0}}}}}}
	records := []reporting.Record{{TypeCode: "ATTENDANCE_RATE", StudentName: "=HYPERLINK(\"https://example.invalid\")", StudentNumber: "001", Grade: "2026级", ClassName: "一班", CurrentCourseName: "课程A"}, {TypeCode: "LATE_RETURN", StudentName: "张三", StudentNumber: "002", Grade: "2026级", ClassName: "一班"}}
	xlsx, err := RenderExcel(content, records, []string{"ATTENDANCE_RATE", "LATE_RETURN"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	names := book.GetSheetList()
	if len(names) != 2 || names[0] != "到课率检查情况" || names[1] != "晚归检查情况" {
		t.Fatalf("sheets=%v", names)
	}
	for _, cell := range []struct {
		address string
		size    float64
		bold    bool
	}{{"A1", 18, true}, {"A2", 12, true}, {"A3", 12, false}} {
		styleID, err := book.GetCellStyle(names[0], cell.address)
		if err != nil {
			t.Fatal(err)
		}
		style, err := book.GetStyle(styleID)
		if err != nil {
			t.Fatal(err)
		}
		if style.Font == nil || style.Font.Family != "宋体" || style.Font.Size != cell.size || style.Font.Bold != cell.bold {
			t.Fatalf("Excel %s font style=%+v", cell.address, style.Font)
		}
	}
	if got, err := book.GetCellValue(names[0], "G2"); err != nil || got != "本周旷课学时" {
		t.Fatalf("Excel header G2=%q, err=%v", got, err)
	}
	if got := excelTypeColumns("ATTENDANCE_RATE"); len(got) != 8 || got[6] != "本周旷课学时" {
		t.Fatalf("preview columns=%v", got)
	}
	value, err := book.GetCellValue(names[0], "C3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(value, "'=HYPERLINK") {
		t.Fatalf("unsafe cell=%q", value)
	}
	formula, err := book.GetCellFormula(names[0], "C3")
	if err != nil {
		t.Fatal(err)
	}
	if formula != "" {
		t.Fatalf("formula injection=%q", formula)
	}
	docx, err := RenderWord(content, records, []string{"ATTENDANCE_RATE", "LATE_RETURN"}, "虚构通报说明", true)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	var document string
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		body, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		document = string(content)
	}
	for _, value := range []string{"虚构通报说明", "二班", "晚归检查情况"} {
		if !strings.Contains(document, value) {
			t.Fatalf("DOCX missing %q", value)
		}
	}
	if !strings.Contains(document, `<w:vMerge w:val="continue"/>`) || !strings.Contains(document, `w:orient="landscape"`) || !strings.Contains(document, `<w:rFonts w:ascii="FangSong" w:eastAsia="仿宋"/><w:sz w:val="24"/>`) || !strings.Contains(document, `<w:rFonts w:ascii="FangSong" w:eastAsia="仿宋"/><w:sz w:val="30"/><w:b/>`) {
		t.Fatal("Word table did not preserve the confirmed grade merge, page orientation, or font")
	}
	if !strings.Contains(document, "HYPERLINK(") || strings.Contains(document, `w:instr="HYPERLINK`) {
		t.Fatal("attendance names should remain plain Word text in the class summary")
	}
}

func TestExcelHygieneSituationAndLateReturnColumns(t *testing.T) {
	content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}}
	records := []reporting.Record{
		{TypeCode: "DORMITORY_HYGIENE", Details: reporting.RecordDetails{Result: "UNQUALIFIED", Description: "地面有垃圾"}},
		{TypeCode: "PUBLIC_DORMITORY_HYGIENE", Details: reporting.RecordDetails{Result: "QUALIFIED", Description: "整洁"}},
		{TypeCode: "CLASSROOM_HYGIENE", Details: reporting.RecordDetails{Result: "EXCELLENT", Description: "桌面整齐"}},
		{TypeCode: "LATE_RETURN", StudentName: "虚构学生", Details: reporting.RecordDetails{Time: "23:30"}},
	}
	data, err := RenderExcel(content, records, []string{"DORMITORY_HYGIENE", "PUBLIC_DORMITORY_HYGIENE", "CLASSROOM_HYGIENE", "LATE_RETURN"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, sample := range []struct{ sheet, cell, want string }{
		{"宿舍卫生检查情况", "C3", "不合格：地面有垃圾"},
		{"宿舍公共卫生检查情况", "C3", "合格：整洁"},
		{"教室卫生检查情况", "F3", "优秀：桌面整齐"},
	} {
		got, err := book.GetCellValue(sample.sheet, sample.cell)
		if err != nil || got != sample.want {
			t.Fatalf("%s %s = %q, err=%v", sample.sheet, sample.cell, got, err)
		}
	}
	for _, code := range []string{"DORMITORY_HYGIENE", "PUBLIC_DORMITORY_HYGIENE", "CLASSROOM_HYGIENE"} {
		for _, title := range excelTypeColumns(code) {
			if title == "原因" {
				t.Fatalf("%s duplicates situation in an explanation column", code)
			}
		}
	}
	for _, title := range excelTypeColumns("LATE_RETURN") {
		if title == "时间" {
			t.Fatal("late return Excel still exposes the removed time column")
		}
	}
	if got, err := book.GetCellValue("晚归检查情况", "G2"); err != nil || got != "备注" {
		t.Fatalf("late return final header = %q, err=%v", got, err)
	}
}

func TestWordAttendanceWithoutIndicatorSnapshotGroupsRecordsByClass(t *testing.T) {
	content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}}
	records := []reporting.Record{
		{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", StudentNumber: "001", CurrentCourseName: "课程甲"},
		{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班", StudentName: "学生乙", StudentNumber: "002", CurrentCourseName: "课程乙"},
	}
	docx, err := RenderWord(content, records, []string{"ATTENDANCE_RATE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		body, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		document := string(encoded)
		if !strings.Contains(document, "应到/实到") || !strings.Contains(document, "学生甲、学生乙") || strings.Count(document, "<w:tr>") != 2 {
			t.Fatal("attendance Word must retain one class row with every student when metrics are unavailable")
		}
		return
	}
	t.Fatal("DOCX document.xml missing")
}

func TestClassRecordNamesKeepsDifferentStudentsWithSameName(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班", StudentName: "同名学生", StudentNumber: "001"},
		{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班", StudentName: "同名学生", StudentNumber: "001"},
		{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班", StudentName: "同名学生", StudentNumber: "002"},
	}
	if got := classRecordNames(records, "ATTENDANCE_RATE", reporting.ReportClassIndicators{Grade: "2026级", ClassName: "一班"}); got != "同名学生、同名学生" {
		t.Fatalf("class student names=%q, want two distinct students", got)
	}
}

func TestWordKeepsSameNamedClassesSeparate(t *testing.T) {
	content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}, Indicators: &reporting.ReportIndicators{Classes: []reporting.ReportClassIndicators{
		{ClassID: "class-a", Grade: "2026级", ClassName: "一班", MajorName: "软件工程"},
		{ClassID: "class-b", Grade: "2026级", ClassName: "一班", MajorName: "网络工程"},
	}}}
	records := []reporting.Record{
		{TypeCode: "ATTENDANCE_RATE", ClassID: "class-a", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", StudentNumber: "001"},
		{TypeCode: "ATTENDANCE_RATE", ClassID: "class-b", Grade: "2026级", ClassName: "一班", StudentName: "学生乙", StudentNumber: "002"},
	}
	classes := reportWordClasses(content, records, "ATTENDANCE_RATE")
	lines := attendanceWordLines(content, records, nil)
	if len(lines) != 2 || len(classes) != 2 || lines[0][1] != "一班（软件工程）" || lines[0][3] != "学生甲" || lines[1][1] != "一班（网络工程）" || lines[1][3] != "学生乙" {
		t.Fatalf("same-named Word classes mixed: %+v", lines)
	}
}

func TestClassRecordAbsenceHoursKeepsMissingValueBlank(t *testing.T) {
	record := reporting.Record{TypeCode: "ATTENDANCE_RATE", Grade: "2026级", ClassName: "一班"}
	if got := classRecordAbsenceHours([]reporting.Record{record}, "ATTENDANCE_RATE", reporting.ReportClassIndicators{Grade: "2026级", ClassName: "一班"}); got != "" {
		t.Fatalf("missing absence hours=%q, want blank", got)
	}
	zero := 0.0
	record.Details.AbsenceHours = &zero
	if got := classRecordAbsenceHours([]reporting.Record{record}, "ATTENDANCE_RATE", reporting.ReportClassIndicators{Grade: "2026级", ClassName: "一班"}); got != "0" {
		t.Fatalf("explicit zero absence hours=%q, want 0", got)
	}
}

func TestWordMorningWithoutIndicatorsGroupsStudentsByClass(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "MORNING_ATTENDANCE", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", StudentNumber: "001"},
		{TypeCode: "MORNING_ATTENDANCE", Grade: "2026级", ClassName: "一班", StudentName: "学生乙", StudentNumber: "002"},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"MORNING_ATTENDANCE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "应到/实到") || !strings.Contains(document, "学生甲、学生乙") || strings.Count(document, "<w:tr>") != 2 {
		t.Fatal("morning Word must retain one class row with both students without indicator data")
	}
}

func TestWordUnattendedGroupsByClassWithoutInventingAttendanceRate(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "UNATTENDED_CLASSROOM", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", StudentNumber: "001", CurrentCourseName: "课程甲"},
		{TypeCode: "UNATTENDED_CLASSROOM", Grade: "2026级", ClassName: "一班", StudentName: "学生乙", StudentNumber: "002", CurrentCourseName: "课程乙"},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"UNATTENDED_CLASSROOM"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "学生甲、学生乙") || strings.Count(document, "<w:tr>") != 2 || strings.Contains(document, "到课率") {
		t.Fatal("unattended Word must use class rows without attendance metrics")
	}
}

func TestWordEveningSelfStudyGroupsStudentsByClass(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "EVENING_SELF_STUDY", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", StudentNumber: "001"},
		{TypeCode: "EVENING_SELF_STUDY", Grade: "2026级", ClassName: "一班", StudentName: "学生乙", StudentNumber: "002"},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"EVENING_SELF_STUDY"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "学生甲、学生乙") || strings.Count(document, "<w:tr>") != 2 || strings.Contains(document, "到课率") {
		t.Fatal("evening self-study Word must use class rows without invented attendance rates")
	}
}

func TestWordLateReturnDoesNotPresentUnrecordedInspectionDenominator(t *testing.T) {
	record := reporting.Record{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", CurrentDormitoryName: "寝室甲"}
	data, err := RenderWord(reporting.ReportVersionContent{}, []reporting.Record{record}, []string{"LATE_RETURN"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if strings.Contains(document, "共查寝间数") || strings.Contains(document, "缺勤率") || !strings.Contains(document, "晚归人员") {
		t.Fatal("late-return Word must omit denominator and rate unavailable in the frozen report")
	}
}

func TestWordPublicDormitoryUsesFrozenResponsibilityLabels(t *testing.T) {
	record := reporting.Record{TypeCode: "PUBLIC_DORMITORY_HYGIENE", Details: reporting.RecordDetails{Date: "2026-11-01", Description: "公共区域杂物"}, ResponsibleDormitories: []reporting.RecordReference{{Label: "责任寝室甲"}}, ResponsibleClasses: []reporting.RecordReference{{Label: "责任班级甲"}}}
	data, err := RenderWord(reporting.ReportVersionContent{}, []reporting.Record{record}, []string{"PUBLIC_DORMITORY_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "责任寝室") || !strings.Contains(document, "责任寝室甲") || !strings.Contains(document, "责任班级甲") || strings.Contains(document, "负责宿舍楼") || strings.Contains(document, "楼层/单元") {
		t.Fatal("public dormitory Word must use the current responsibility snapshot rather than historical area columns")
	}
}

func TestWordDetailRowsFollowGradeClassDateOrder(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "二班", StudentName: "学生乙", Details: reporting.RecordDetails{Date: "2026-11-02"}},
		{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "一班", StudentName: "学生甲", Details: reporting.RecordDetails{Date: "2026-11-01"}},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"LATE_RETURN"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if strings.Index(document, "学生甲") >= strings.Index(document, "学生乙") {
		t.Fatal("Word detail rows must be ordered by grade and class before source ID")
	}
}

func TestWordDormitorySeparatesExcellentRoomsFromUnqualifiedDetails(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "DORMITORY_HYGIENE", Grade: "2026级", ClassName: "一班", CurrentDormitoryName: "优秀寝室甲", Details: reporting.RecordDetails{Result: "EXCELLENT"}},
		{TypeCode: "DORMITORY_HYGIENE", Grade: "2026级", ClassName: "二班", CurrentDormitoryName: "不合格寝室乙", Details: reporting.RecordDetails{Result: "UNQUALIFIED"}},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"DORMITORY_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "优秀寝室") || !strings.Contains(document, "不合格寝室") || strings.Count(document, "<w:tbl>") != 2 {
		t.Fatal("dormitory Word must show separate excellent and unqualified tables")
	}
}

func TestWordDormitoryRetainsQualifiedAndLegacyUnknownResults(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "合格寝室甲", Details: reporting.RecordDetails{Result: "QUALIFIED"}},
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "历史寝室乙", Details: reporting.RecordDetails{Result: "LEGACY"}},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"DORMITORY_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "合格寝室甲") || !strings.Contains(document, "历史寝室乙") {
		t.Fatal("dormitory Word must retain all frozen results, including legacy values")
	}
}

func TestWordExcellentDormitoryListsAllResponsibleClasses(t *testing.T) {
	record := reporting.Record{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "优秀寝室甲", ClassName: "一班", Details: reporting.RecordDetails{Result: "EXCELLENT"}, ResponsibleClasses: []reporting.RecordReference{{Label: "一班"}, {Label: "二班"}}}
	data, err := RenderWord(reporting.ReportVersionContent{}, []reporting.Record{record}, []string{"DORMITORY_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if document := wordDocumentForTest(t, data); !strings.Contains(document, "一班、二班") {
		t.Fatal("excellent dormitory Word must list every frozen responsible class")
	}
}

func TestWordExcellentDormitoryPairsTwoRoomsPerRow(t *testing.T) {
	records := []reporting.Record{
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "优秀寝室甲", Details: reporting.RecordDetails{Result: "EXCELLENT"}},
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "优秀寝室乙", Details: reporting.RecordDetails{Result: "EXCELLENT"}},
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "优秀寝室丙", Details: reporting.RecordDetails{Result: "EXCELLENT"}},
	}
	data, err := RenderWord(reporting.ReportVersionContent{}, records, []string{"DORMITORY_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if strings.Count(document, "<w:tr>") != 3 || strings.Count(document, ">寝室号<") != 2 {
		t.Fatal("excellent dormitory Word must use paired eight-column rows and retain odd final room")
	}
	for _, dormitory := range []string{"优秀寝室甲", "优秀寝室乙", "优秀寝室丙"} {
		if !strings.Contains(document, dormitory) {
			t.Fatalf("missing excellent dormitory %s", dormitory)
		}
	}
}

func TestWordDormitoryPreviewSectionsMatchGeneratedHeaders(t *testing.T) {
	_, columns := outputTypeLayout("DORMITORY_HYGIENE")
	sections := wordOutputSections("DORMITORY_HYGIENE", columns)
	if len(sections) != 4 || sections[0].Title != "不合格寝室" || sections[1].Title != "优秀寝室" || sections[2].Title != "合格寝室" || sections[3].Title != "其他宿舍检查记录" {
		t.Fatalf("unexpected dormitory preview sections: %+v", sections)
	}
	if len(sections[1].Columns) != 8 || sections[1].Columns[0] != "寝室号" || sections[1].Columns[4] != "寝室号" {
		t.Fatalf("excellent dormitory preview must show two room groups: %+v", sections[1].Columns)
	}
}

func TestWordClassroomHygieneShowsFrozenResultWithDescription(t *testing.T) {
	record := reporting.Record{TypeCode: "CLASSROOM_HYGIENE", Grade: "2026级", ClassName: "一班", Details: reporting.RecordDetails{Result: "UNQUALIFIED", Description: "地面有纸屑"}}
	data, err := RenderWord(reporting.ReportVersionContent{}, []reporting.Record{record}, []string{"CLASSROOM_HYGIENE"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if document := wordDocumentForTest(t, data); !strings.Contains(document, "不合格：地面有纸屑") {
		t.Fatal("classroom Word must show the frozen check result as well as its description")
	}
}

func wordDocumentForTest(t *testing.T, data []byte) string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		body, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	t.Fatal("DOCX document.xml missing")
	return ""
}

func TestEmptyOutputKeepsUnsubmittedDimensionDistinctFromConfirmedZero(t *testing.T) {
	for _, source := range []struct{ status, want string }{
		{status: "NO_CONFIRMED_DATA", want: "来源未完成，暂无已确认数据"},
		{status: "CONFIRMED_ZERO_RECORD_RECEIPT", want: "本周无已确认违纪记录"},
	} {
		content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}, Rows: []reporting.ReportCountRow{{InspectionTypeCode: "PUBLIC_DORMITORY_HYGIENE", DataStatus: source.status}}}
		xlsx, err := RenderExcel(content, nil, []string{"PUBLIC_DORMITORY_HYGIENE"})
		if err != nil {
			t.Fatal(err)
		}
		book, err := excelize.OpenReader(bytes.NewReader(xlsx))
		if err != nil {
			t.Fatal(err)
		}
		value, cellErr := book.GetCellValue(book.GetSheetName(0), "A3")
		book.Close()
		if cellErr != nil || value != source.want {
			t.Fatalf("%s Excel empty row=%q err=%v", source.status, value, cellErr)
		}
		docx, err := RenderWord(content, nil, []string{"PUBLIC_DORMITORY_HYGIENE"}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, part := range archive.File {
			if part.Name != "word/document.xml" {
				continue
			}
			body, openErr := part.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			contents, readErr := io.ReadAll(body)
			body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			found = strings.Contains(string(contents), source.want) && strings.Contains(string(contents), "第八周公共宿舍卫生检查通报")
		}
		if !found {
			t.Fatalf("%s Word omitted the empty public dormitory hygiene dimension", source.status)
		}
	}
}

func TestWordAttachmentColumnsUseAvailableFrozenFields(t *testing.T) {
	cases := map[string][]string{
		"ATTENDANCE_RATE":          {"年级", "班级", "应到/实到", "缺课同学", "课程", "班主任", "缺课总课时", "到课率"},
		"CLASSROOM_HYGIENE":        {"年级", "日期", "班级", "负责人员", "班主任", "教室", "检查情况"},
		"DORMITORY_HYGIENE":        {"日期", "寝室号", "情况", "责任人", "所在班级", "班主任"},
		"PUBLIC_DORMITORY_HYGIENE": {"日期", "责任寝室", "情况", "责任班级", "备注"},
		"LATE_RETURN":              {"年级", "日期", "班级", "寝室号", "晚归人员", "班主任", "备注"},
		"MORNING_ATTENDANCE":       {"年级", "班级", "应到/实到", "缺到人员", "班主任", "缺勤总次数", "出勤率"},
		"UNATTENDED_CLASSROOM":     {"年级", "班级", "违纪学生", "课程", "班主任", "违规次数"},
		"EVENING_SELF_STUDY":       {"年级", "班级", "缺勤学生", "班主任", "缺勤次数"},
	}
	for code, want := range cases {
		_, got := outputTypeLayout(code)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s columns=%v, want %v", code, got, want)
		}
	}
}

func TestWordUsesFrozenTeacherWithoutInventingDormitoryCount(t *testing.T) {
	content := reporting.ReportVersionContent{Indicators: &reporting.ReportIndicators{Inputs: reporting.ReportMetricInputs{Classes: []reporting.ReportClassSizeSnapshot{{Grade: "2026级", ClassName: "一班", HeadTeacherName: "Fixture teacher"}}}}}
	records := []reporting.Record{{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "一班", StudentName: "Fixture student", CurrentDormitoryName: "寝室 101"}}
	late := recordColumns()[6]
	columns := wordRecordColumns(late, headTeacherNames(content))
	if got := columns[5].value(records[0]); got != "Fixture teacher" {
		t.Fatalf("frozen teacher=%q", got)
	}
	docx, err := RenderWord(content, records, []string{"LATE_RETURN"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		body, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Fixture teacher") || strings.Contains(string(data), "共查寝间数") || strings.Contains(string(data), "缺勤率") {
			t.Fatal("DOCX must use the frozen teacher without an unrecorded dormitory denominator")
		}
		return
	}
	t.Fatal("DOCX document.xml missing")
}
func TestRenderRejectsInvalidSelection(t *testing.T) {
	_, err := RenderExcel(reporting.ReportVersionContent{}, nil, []string{"UNKNOWN"})
	if err != ErrInvalidSelection {
		t.Fatalf("got %v", err)
	}
	_, err = RenderWord(reporting.ReportVersionContent{}, nil, []string{"LATE_RETURN", "LATE_RETURN"}, "", false)
	if err != ErrInvalidSelection {
		t.Fatalf("got %v", err)
	}
}
