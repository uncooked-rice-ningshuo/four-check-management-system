package reportoutput

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/reporting"
)

func TestIntroductionFormattingSurvivesDocxMapping(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","headerText":"学院页首","footerText":"办公室页尾","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"左侧正文","marks":[{"type":"textStyle","attrs":{"fontFamily":"仿宋","fontSize":"15pt"}}]}]},{"type":"paragraph","attrs":{"style":"signature","textAlign":"right"},"content":[{"type":"text","text":"办公室","marks":[{"type":"bold"},{"type":"italic"},{"type":"strike"},{"type":"underline"}]}]}]}`)
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, nil, []string{"LATE_RETURN"}, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" && file.Name != "word/styles.xml" && file.Name != "word/header1.xml" && file.Name != "word/footer1.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[file.Name] = string(content)
	}
	for _, expected := range []string{`w:val="left"`, `w:val="right"`, `w:eastAsia="仿宋"`, `w:val="30"`, `<w:b/>`, `<w:i/>`, `<w:strike/>`, `<w:u w:val="single"/>`, `w:orient="landscape"`} {
		if !strings.Contains(parts["word/document.xml"], expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	if !strings.Contains(parts["word/styles.xml"], `w:styleId="CoverBody"`) {
		t.Fatal("missing cover style")
	}
	if !strings.Contains(parts["word/header1.xml"], "学院页首") || !strings.Contains(parts["word/footer1.xml"], "办公室页尾") {
		t.Fatal("missing configured page header or footer")
	}
}

func TestIntroductionRejectsUnsupportedFormatting(t *testing.T) {
	for _, raw := range []string{
		`{"type":"doc","content":[{"type":"image","attrs":{"style":"body","textAlign":"left"}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"x","marks":[{"type":"textStyle","attrs":{"fontFamily":"任意字体"}}]}]}]}`,
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left","html":"<script>"}}]}`,
		`{"type":"doc","headerText":"换行\n注入","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`,
		`{"type":"doc","titleTemplate":"{unknown}","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`,
		`{"type":"doc","attachmentPosition":2,"content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`,
	} {
		if _, err := decodeIntroduction(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestCoverTitleAndAttachmentPositionSurviveDocx(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","titleTemplate":"{semesterName} 第{weekNo}周专项通报","attachmentPosition":1,"content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"目录之前"}]},{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"目录之后"}]}]}`)
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, nil, []string{"LATE_RETURN"}, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		xml := string(content)
		if !strings.Contains(xml, "示例学期 第9周专项通报") || !(strings.Index(xml, "目录之前") < strings.Index(xml, "附件1：晚归检查情况") && strings.Index(xml, "附件1：晚归检查情况") < strings.Index(xml, "目录之后")) {
			t.Fatal("title or attachment order was not rendered")
		}
		return
	}
	t.Fatal("document.xml missing")
}

func TestIntroductionDateIsResolvedForGeneratedWord(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"signature","textAlign":"right"},"content":[{"type":"text","text":"学院学工办"}]},{"type":"paragraph","attrs":{"style":"date","textAlign":"right"},"content":[{"type":"text","text":"{reportDate}"}]}]}`)
	for _, date := range []string{"", "2025-02-30", "2025/06/01"} {
		if _, _, err := resolveIntroductionDate(raw, date); err == nil {
			t.Fatalf("invalid issue date %q accepted", date)
		}
	}
	resolved, issueDate, err := resolveIntroductionDate(raw, "2025-06-01")
	if err != nil || issueDate != "2025-06-01" || !strings.Contains(string(resolved), "2025年6月1日") || strings.Contains(string(resolved), "{reportDate}") {
		t.Fatalf("date was not resolved: issueDate=%q err=%v", issueDate, err)
	}
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, nil, []string{"LATE_RETURN"}, resolved, true)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "2025年6月1日") || strings.Contains(string(content), "{reportDate}") {
			t.Fatal("generated Word did not contain selected date")
		}
		return
	}
	t.Fatal("document.xml missing")
}

func TestEmptyTitleFontSize(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"title","fontSize":"15pt","textAlign":"center","lineSpacingMode":"single"}}]}`)
	doc, err := decodeIntroduction(raw)
	if err != nil {
		t.Fatal(err)
	}
	if xml := renderIntroduction(doc); !strings.Contains(xml, `<w:rPr><w:sz w:val="30"/></w:rPr></w:pPr></w:p>`) {
		t.Fatal(xml)
	}
}

func TestIntroductionLineSpacingModes(t *testing.T) {
	for _, test := range []struct{ mode, rule string }{
		{"single", `w:line="240" w:lineRule="auto"`},
		{"oneHalf", `w:line="360" w:lineRule="auto"`},
		{"double", `w:line="480" w:lineRule="auto"`},
		{"fixed", `w:line="540" w:lineRule="exact"`},
	} {
		raw := json.RawMessage(fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left","lineSpacing":27,"lineSpacingMode":%q},"content":[{"type":"text","text":"示例"}]}]}`, test.mode))
		document, err := decodeIntroduction(raw)
		if err != nil {
			t.Fatal(err)
		}
		if xml := renderIntroduction(document); !strings.Contains(xml, test.rule) {
			t.Fatalf("%s: %s", test.mode, xml)
		}
	}
	if _, err := decodeIntroduction(json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left","lineSpacingMode":"unknown"},"content":[{"type":"text","text":"示例"}]}]}`)); err == nil {
		t.Fatal("unsupported line spacing mode accepted")
	}
}

func TestWordFooterTextAndPageNumberHaveSeparateParagraphs(t *testing.T) {
	xml := wordFooterXML("办公室页尾")
	if strings.Count(xml, "<w:p>") != 2 || strings.Index(xml, "办公室页尾") > strings.Index(xml, `w:instr="PAGE"`) {
		t.Fatal("footer text and page number must occupy separate lines")
	}
}

func TestWordFooterInlineAndHiddenPageNumber(t *testing.T) {
	inline := wordFooterLayoutXML("办公室页尾", "inline")
	if !strings.Contains(inline, `<w:tbl>`) || !strings.Contains(inline, `<w:jc w:val="left"/>`) || !strings.Contains(inline, `<w:jc w:val="right"/>`) || !strings.Contains(inline, `w:instr="PAGE"`) {
		t.Fatal("inline footer must place text left and page number right")
	}
	if strings.Contains(wordFooterLayoutXML("办公室页尾", "textOnly"), `w:instr="PAGE"`) {
		t.Fatal("text-only footer must hide the page number")
	}
	if _, err := decodeIntroduction(json.RawMessage(`{"type":"doc","footerLayout":"diagonal","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`)); err == nil {
		t.Fatal("unsupported footer layout accepted")
	}
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, nil, []string{"LATE_RETURN"}, json.RawMessage(`{"type":"doc","footerText":"办公室页尾","footerLayout":"inline","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneratedFile("WORD", data); err != nil {
		t.Fatal(err)
	}
}

func TestWordFooterCustomEditableField(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","footerLayout":"custom","footerAlign":"left","footerText":"学院办公室\t{page}","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`)
	document, err := decodeIntroduction(raw)
	if err != nil {
		t.Fatal(err)
	}
	footer := wordFooterDocumentXML(document, "示例学期", 9)
	if !strings.Contains(footer, "学院办公室") || !strings.Contains(footer, `<w:tbl>`) || !strings.Contains(footer, `w:instr="PAGE"`) {
		t.Fatalf("custom footer lost text or page field: %s", footer)
	}
	data, err := RenderWordDocument(reporting.ReportVersionContent{}, nil, []string{"LATE_RETURN"}, raw, true)
	if err != nil || ValidateGeneratedFile("WORD", data) != nil {
		t.Fatalf("custom footer did not produce valid Word: %v", err)
	}
	threeZones := json.RawMessage(`{"type":"doc","footerLayout":"custom","footerText":"{semesterName} 第{weekNo}周\t第 {page} 页 共 {totalPages} 页\t","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`)
	threeZoneDocument, err := decodeIntroduction(threeZones)
	if err != nil {
		t.Fatal(err)
	}
	threeZoneFooter := wordFooterDocumentXML(threeZoneDocument, "2025-2026 二学期", 8)
	for _, expected := range []string{"2025-2026 二学期", "第8周", `w:instr="PAGE"`, `w:instr="NUMPAGES"`, `<w:tbl>`} {
		if !strings.Contains(threeZoneFooter, expected) {
			t.Fatalf("footer missing %q: %s", expected, threeZoneFooter)
		}
	}
	threeZoneFile, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "2025-2026 二学期", WeekNo: 8}}, nil, []string{"LATE_RETURN"}, threeZones, true)
	if err != nil || ValidateGeneratedFile("WORD", threeZoneFile) != nil {
		t.Fatalf("three-zone footer file invalid: %v", err)
	}
	if path := os.Getenv("REPORT_OUTPUT_FOOTER_SAMPLE_PATH"); path != "" {
		if err := os.WriteFile(path, threeZoneFile, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{`{"type":"doc","footerLayout":"custom","footerText":"{page}{page}","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`, `{"type":"doc","footerLayout":"custom","footerAlign":"diagonal","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`, `{"type":"doc","footerLayout":"custom","footerText":"左\t中\t右\t额外","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"}}]}`} {
		if _, err := decodeIntroduction(json.RawMessage(invalid)); err == nil {
			t.Fatal("invalid footer accepted")
		}
	}
}

func TestWordAcceptsChineseWeekAndPageVariables(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","footerLayout":"custom","footerText":"第{pageChinese}页，共{totalPagesChinese}页 · 第{weekNoChinese}周","content":[{"type":"paragraph","attrs":{"style":"subtitle","textAlign":"center"},"content":[{"type":"text","text":"第{weekNoChinese}周日常管理检查通报"}]}]}`)
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 8}}, nil, []string{"ATTENDANCE_RATE"}, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	document := wordDocumentForTest(t, data)
	if !strings.Contains(document, "第八周日常管理检查通报") || !strings.Contains(document, "第八周到课率检查通报") {
		t.Fatal("Chinese week missing from cover or attachment title")
	}
	if path := os.Getenv("REPORT_OUTPUT_CHINESE_SAMPLE_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManualAttachmentTextAndHeadingStyle(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","manualAttachmentList":true,"content":[{"type":"paragraph","attrs":{"style":"title","textAlign":"center"},"content":[{"type":"text","text":"计算机科学与工程学院"}]},{"type":"paragraph","attrs":{"style":"subtitle","textAlign":"center","spaceBefore":8},"content":[{"type":"text","text":"{semesterName} 第{weekNo}周通报"}]},{"type":"paragraph","attrs":{"style":"heading2","textAlign":"left","indent":2,"spaceBefore":16,"lineSpacing":25},"content":[{"type":"text","text":"自定义附件说明","marks":[{"type":"textStyle","attrs":{"fontFamily":"楷体"}}]}]}]}`)
	data, err := RenderWordDocument(reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}, nil, []string{"LATE_RETURN"}, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		xml := string(content)
		if !strings.Contains(xml, `w:pStyle w:val="CoverHeading2"`) || !strings.Contains(xml, `w:eastAsia="楷体"`) || !strings.Contains(xml, `w:ind w:firstLineChars="200"`) || !strings.Contains(xml, `w:spacing w:before="320" w:after="0" w:line="500"`) || !strings.Contains(xml, "示例学期 第9周通报") || strings.Contains(xml, "日常管理通报") || strings.Contains(xml, "附件：1.") {
			t.Fatal("manual text, heading or font mismatch")
		}
		return
	}
	t.Fatal("document.xml missing")
}

func TestTemplateSectionsAndEmbeddedTable(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":2,"content":[{"type":"paragraph","attrs":{"style":"title","textAlign":"center"},"content":[{"type":"text","text":"自拟表题"}]},{"type":"pageBreak"},{"type":"sectionBreak","attrs":{"orientation":"landscape"}},{"type":"reportTable","attrs":{"key":"ATTENDANCE_RATE"}},{"type":"sectionBreak","attrs":{"orientation":"portrait"}},{"type":"paragraph","attrs":{"style":"signature","textAlign":"right"},"content":[{"type":"templateField","attrs":{"key":"report.semesterName"}}]}]}`)
	snapshot := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "测试学期", WeekNo: 3}}
	data, err := RenderWordDocument(snapshot, nil, []string{"ATTENDANCE_RATE"}, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneratedFile("WORD", data); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("REPORT_OUTPUT_SECTION_SAMPLE_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	part, err := archive.Open("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer part.Close()
	body, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(body)
	for token, count := range map[string]int{"<w:sectPr>": 3, `w:orient="landscape"`: 1, `w:type="page"`: 1, "自拟表题": 1, "<w:tbl>": 1, "测试学期": 1} {
		if strings.Count(xml, token) != count {
			t.Errorf("unexpected count for %q", token)
		}
	}
	if strings.Index(xml, "自拟表题") > strings.Index(xml, "<w:tbl>") || strings.Index(xml, "测试学期") < strings.Index(xml, "</w:tbl>") {
		t.Error("template order lost")
	}
	if _, err := RenderWordDocument(snapshot, nil, []string{"LATE_RETURN"}, raw, true); err == nil {
		t.Error("rendered a table outside the selected inspection types")
	}
	invalid := strings.Replace(string(raw), "report.semesterName", "discipline.decidedAt", 1)
	if _, err := decodeIntroduction(json.RawMessage(invalid)); err == nil {
		t.Error("accepted a discipline-only field")
	}
}

func TestDisciplineTemplateSources(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":2,"orientation":"landscape","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left","lineSpacingMode":"fixed","lineSpacing":0},"content":[{"type":"templateField","attrs":{"key":"discipline.studentName"}}]}]}`)
	for name, expected := range map[string]string{"示例甲": "示例甲", "王宁": "王　宁", "王  宁": "王　宁", "王　宁": "王　宁"} {
		data, err := RenderDisciplineDocument(raw, map[string]string{"discipline.studentName": name}, time.Unix(0, 0))
		if err != nil {
			t.Fatal(err)
		}
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		r, err := z.Open("word/document.xml")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if !strings.Contains(string(body), `w:line="240" w:lineRule="auto"`) {
			t.Error("unset discipline body line height should follow the font")
		}
		if err != nil || !strings.Contains(string(body), expected) || !strings.Contains(string(body), `w:orient="landscape"`) {
			t.Fatalf("missing case name or orientation: %v", err)
		}
	}
	if _, err := RenderDisciplineDocument(raw, nil, time.Unix(0, 0)); err == nil {
		t.Fatal("missing student accepted")
	}
	for _, illegal := range []string{`{"schemaVersion":2,"content":[{"type":"reportTable","attrs":{"key":"ATTENDANCE_RATE"}}]}`, strings.Replace(string(raw), "discipline.studentName", "report.reportDate", 1)} {
		if err := ValidateDisciplineTemplate(json.RawMessage(illegal)); err == nil {
			t.Fatal("foreign node accepted")
		}
	}
}

func TestTemplateParagraphBorders(t *testing.T) {
	for _, tc := range []struct{ border, want string }{{"topBottom", "<w:top "}, {"bottomDouble", `w:val="double"`}} {
		raw := json.RawMessage(`{"schemaVersion":2,"content":[{"type":"paragraph","attrs":{"border":"` + tc.border + `"},"content":[{"type":"text","text":"印发"}]}]}`)
		doc, err := decodeTemplate(raw, true)
		if err != nil {
			t.Fatal(err)
		}
		xml := renderIntroduction(doc)
		if !strings.Contains(xml, "<w:pBdr>") || !strings.Contains(xml, tc.want) || !strings.Contains(xml, "<w:bottom ") {
			t.Fatalf("missing %s in %s", tc.border, xml)
		}
	}
	sample := json.RawMessage(`{"schemaVersion":2,"content":[{"type":"paragraph","attrs":{"border":"topBottom"},"content":[{"type":"text","text":"示例办公室　　　　　　2026年11月23日印发"}]},{"type":"paragraph"},{"type":"paragraph","attrs":{"border":"bottomDouble"},"content":[{"type":"text","text":"示例落款"}]}]}`)
	data, err := RenderDisciplineDocument(sample, nil, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneratedFile("WORD", data); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("DISCIPLINE_BORDER_SAMPLE_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateDisciplineTemplate(json.RawMessage(`{"schemaVersion":2,"content":[{"type":"paragraph","attrs":{"border":"invalid"}}]}`)); err == nil {
		t.Fatal("accepted invalid border")
	}
}

func TestImprintTableCells(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":2,"content":[{"type":"imprintTable","attrs":{"cellSplit":1},"content":[{"type":"text","text":"印发单位"},{"type":"templateField","attrs":{"key":"discipline.issuedDate"}}]}]}`)
	doc, err := decodeTemplate(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderDisciplineDocument(raw, map[string]string{"discipline.issuedDate": "示例日期"}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	var xml strings.Builder
	writeImprintTable(&xml, doc.Content[0], 10106)
	for _, part := range []string{"<w:tbl>", `<w:insideV w:val="nil"`, `<w:jc w:val="left"/>`, `<w:jc w:val="right"/>`} {
		if !strings.Contains(xml.String(), part) {
			t.Fatal(part)
		}
	}
	if ValidateDisciplineTemplate(json.RawMessage(strings.Replace(string(raw), `"cellSplit":1`, `"cellSplit":3`, 1))) == nil {
		t.Fatal("invalid cell split accepted")
	}
}

func TestGenericTableDimensionsAndFields(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":2,"content":[{"type":"wordTable","attrs":{"border":"none","insetLeft":1000,"insetRight":1500,"rows":[[{"length":0,"align":"left","height":48,"fontSize":"9pt"},{"length":1,"align":"center","verticalAlign":"bottom","width":4500,"hiddenEdges":8}],[{"length":0,"align":"right"},{"length":0,"align":"left"}]]},"content":[{"type":"templateField","attrs":{"key":"discipline.tier"}}]}]}`)
	data, err := RenderDisciplineDocument(raw, map[string]string{"discipline.tier": "示例档位"}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := z.Open("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	xml, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(xml, []byte("<w:tc>")) != 4 || !bytes.Contains(xml, []byte("示例档位")) || !bytes.Contains(xml, []byte(`w:jc w:val="center"`)) {
		t.Fatal("table layout or field lost")
	}
	for _, part := range []string{`<w:vAlign w:val="bottom"/>`, `<w:rPr><w:sz w:val="18"/></w:rPr></w:pPr></w:p>`, `w:line="240" w:lineRule="auto"`, `<w:tblW w:type="pct" w:w="3750"/>`, `<w:tblInd w:type="dxa" w:w="1010"/>`, `<w:trHeight w:val="960" w:hRule="atLeast"/>`, `<w:tcW w:type="pct" w:w="2250"/>`, `<w:left w:val="nil"/>`} {
		if !bytes.Contains(xml, []byte(part)) {
			t.Fatal(part)
		}
	}
	if ValidateDisciplineTemplate(json.RawMessage(strings.Replace(string(raw), `"verticalAlign":"bottom"`, `"verticalAlign":"baseline"`, 1))) == nil {
		t.Fatal("invalid vertical alignment accepted")
	}
	if ValidateDisciplineTemplate(json.RawMessage(strings.Replace(string(raw), "9pt", "999pt", 1))) == nil {
		t.Fatal("invalid cell font size accepted")
	}
	if ValidateDisciplineTemplate(json.RawMessage(strings.Replace(string(raw), `"length":1`, `"length":2`, 1))) == nil {
		t.Fatal("out of range cell length")
	}
}
