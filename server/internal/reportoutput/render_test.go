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
			found = strings.Contains(string(contents), source.want) && strings.Contains(string(contents), "宿舍公共卫生检查情况")
		}
		if !found {
			t.Fatalf("%s Word omitted the empty public dormitory hygiene dimension", source.status)
		}
	}
}

func TestWordAttachmentColumnsFollowReferenceTables(t *testing.T) {
	cases := map[string][]string{
		"ATTENDANCE_RATE":          {"年级", "班级", "应到/实到", "缺课同学", "课程", "班主任", "缺课总课时", "到课率"},
		"CLASSROOM_HYGIENE":        {"年级", "日期", "班级", "负责人员", "班主任", "教室", "违纪情况"},
		"DORMITORY_HYGIENE":        {"日期", "寝室号", "情况", "责任人", "所在班级", "班主任"},
		"PUBLIC_DORMITORY_HYGIENE": {"年级", "日期", "班级", "寝室号", "班主任", "负责宿舍楼", "楼层/单元"},
		"LATE_RETURN":              {"年级", "日期", "班级", "寝室号", "晚归人员", "班主任", "共查寝间数", "缺勤率", "备注"},
		"MORNING_ATTENDANCE":       {"年级", "班级", "应到/实到", "缺到人员", "班主任", "缺勤总次数", "出勤率"},
	}
	for code, want := range cases {
		_, got := outputTypeLayout(code)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s columns=%v, want %v", code, got, want)
		}
	}
}

func TestWordUsesFrozenTeacherAndWeeklyDistinctDormitories(t *testing.T) {
	content := reporting.ReportVersionContent{Indicators: &reporting.ReportIndicators{Inputs: reporting.ReportMetricInputs{Classes: []reporting.ReportClassSizeSnapshot{{Grade: "2026级", ClassName: "一班", HeadTeacherName: "Fixture teacher"}}}}}
	records := []reporting.Record{
		{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "一班", StudentName: "Fixture student", CurrentDormitoryName: "寝室 101"},
		{TypeCode: "DORMITORY_HYGIENE", CurrentDormitoryName: "寝室 101", ResponsibleDormitories: []reporting.RecordReference{{Label: "寝室 102"}}},
		{TypeCode: "PUBLIC_DORMITORY_HYGIENE", ResponsibleDormitories: []reporting.RecordReference{{Label: "寝室 102"}, {Label: "寝室 103"}}},
	}
	if got := checkedDormitoryCount(records); got != 3 {
		t.Fatalf("distinct dormitories=%d, want 3", got)
	}
	late := recordColumns()[6]
	columns := wordRecordColumns(late, headTeacherNames(content), checkedDormitoryCount(records))
	if got := columns[5].value(records[0]); got != "Fixture teacher" {
		t.Fatalf("frozen teacher=%q", got)
	}
	if got := columns[6].value(records[0]); got != "3" {
		t.Fatalf("weekly inspected dormitories=%q", got)
	}
	if got := columns[7].value; got != nil {
		t.Fatal("attendance rate must remain blank until its calculation is defined")
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
		if !strings.Contains(string(data), "Fixture teacher") || !strings.Contains(string(data), "共查寝间数") || !strings.Contains(string(data), ">3<") {
			t.Fatal("DOCX missing frozen teacher or distinct weekly dormitory count")
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
