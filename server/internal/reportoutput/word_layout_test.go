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
	if tables < 2 || tables > 8 || strings.Count(document, "<w:tblHeader/>") != tables {
		t.Fatalf("unexpected page grouping: tables=%d headers=%d", tables, strings.Count(document, "<w:tblHeader/>"))
	}
	if !strings.Contains(parts["word/settings.xml"], `w:val="15"`) || !strings.Contains(parts["word/footer1.xml"], `w:instr="PAGE"`) {
		t.Fatal("missing modern compatibility mode or page footer")
	}
}
