package reportoutput

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"four-check-management-system/server/internal/reporting"
)

func TestLongRemarkLayoutReservesSpaceAndRepeatsHeaders(t *testing.T) {
	columns := []column{{title: "年级"}, {title: "班级"}, {title: "姓名"}, {title: "学号"}, {title: "日期"}, {title: "寝室"}, {title: "时间"}, {title: "备注"}}
	widths := wordColumnWidths(columns)
	total := 0
	for _, width := range widths {
		total += width
	}
	if total != 15000 || widths[7] <= widths[1] || widths[1] <= widths[0] {
		t.Fatalf("unexpected column widths: %v", widths)
	}
	records := make([]reporting.Record, 40)
	for index := range records {
		records[index] = reporting.Record{TypeCode: "LATE_RETURN", Grade: "2026级", ClassName: "虚构测试班", Details: reporting.RecordDetails{Remark: strings.Repeat("虚构长文本", 8)}}
	}
	data, err := RenderWord(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, records, []string{"LATE_RETURN"}, "示例说明", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneratedFile("WORD", data); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("REPORT_OUTPUT_SAMPLE_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, part := range archive.File {
		if part.Name != "word/document.xml" && part.Name != "word/settings.xml" && part.Name != "word/footer1.xml" {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[part.Name] = string(content)
	}
	document := parts["word/document.xml"]
	positions := []int{
		strings.Index(document, "示例说明"),
		strings.Index(document, "附件1：晚归检查情况"),
		strings.Index(document, `<w:type w:val="nextPage"/>`),
		strings.Index(document, "<w:tbl>"),
	}
	for index, position := range positions {
		if position < 0 || index > 0 && position <= positions[index-1] {
			t.Fatalf("cover or attachment order is wrong: %v", positions)
		}
	}
	tables := strings.Count(document, "<w:tbl>")
	if tables < 2 || tables > 12 || strings.Count(document, "<w:tblHeader/>") != tables {
		t.Fatalf("unexpected page grouping: tables=%d headers=%d", tables, strings.Count(document, "<w:tblHeader/>"))
	}
	if !strings.Contains(parts["word/settings.xml"], `w:val="15"`) || !strings.Contains(parts["word/footer1.xml"], `w:instr="PAGE"`) {
		t.Fatal("missing modern compatibility mode or page footer")
	}
}

func TestWordAttachmentTitleAndTableFormattingMatchReference(t *testing.T) {
	if got := wordAttachmentTitle("ATTENDANCE_RATE", 8); got != "第八周到课率检查通报" {
		t.Fatalf("attachment title=%q", got)
	}
	if got := chineseNumber(12); got != "十二" {
		t.Fatalf("Chinese week=%q", got)
	}
	columns := []column{{title: "年级"}, {title: "缺课同学"}, {title: "课程"}, {title: "缺课总课时"}}
	var document strings.Builder
	renderWordTablePages(&document, wordAttachmentTitle("ATTENDANCE_RATE", 8), "ATTENDANCE_RATE", columns, [][]string{{"2026级", "", "", ""}}, false)
	xml := document.String()
	if strings.Count(xml, `>无</w:t>`) != 2 || !strings.Contains(xml, `<w:vAlign w:val="center"/>`) || !strings.Contains(xml, `<w:trHeight w:val="600" w:hRule="atLeast"/>`) || !strings.Contains(xml, `<w:tblHeader/>`) {
		t.Fatalf("Word table did not match reference empty values, alignment, row height or repeated header: %s", xml)
	}
	if strings.Contains(xml, `>无</w:t></w:r></w:p></w:tc></w:tr>`) {
		t.Fatal("missing numeric metric was changed to 无")
	}
}

func TestOversizedWordRowGetsItsOwnPageAndFollowingRowsContinue(t *testing.T) {
	widths := []int{1000, 1000}
	big := []string{"2026级", strings.Repeat("虚构学生", 70)}
	lines := [][]string{{"2026级", "短"}, big, {"2026级", "短二"}, {"2026级", "短三"}}
	pages := paginateWordRows(lines, widths)
	if len(pages) != 3 || len(pages[0]) != 1 || len(pages[1]) != 1 || len(pages[2]) != 2 {
		t.Fatalf("oversized row pagination=%v", pages)
	}
}

func TestChinesePageFieldsRemainDynamic(t *testing.T) {
	document := IntroductionDocument{FooterLayout: "custom", FooterText: "第{pageChinese}页，共{totalPagesChinese}页 · 第{weekNoChinese}周"}
	footer := wordFooterDocumentXML(document, "示例学期", 8)
	if !strings.Contains(footer, `w:instr="PAGE \* CHINESENUM1"`) || !strings.Contains(footer, `w:instr="NUMPAGES \* CHINESENUM1"`) || !strings.Contains(footer, "第八周") {
		t.Fatalf("Chinese fields not rendered: %s", footer)
	}
	if wordPageNumberFormat(document) != `<w:pgNumType w:fmt="chineseCounting"/>` {
		t.Fatal("section numbering format missing")
	}
	mixed := wordFooterDocumentXML(IntroductionDocument{FooterLayout: "custom", FooterText: "{pageChinese} / {totalPages}"}, "示例学期", 8)
	if !strings.Contains(mixed, `w:instr="NUMPAGES \* Arabic"`) || !strings.Contains(mixed, `w:instr="PAGE \* CHINESENUM1"`) {
		t.Fatal("mixed page number formats lost")
	}
}
