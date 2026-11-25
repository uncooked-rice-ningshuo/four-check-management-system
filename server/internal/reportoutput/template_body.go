package reportoutput

import (
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/reporting"
)

func validOrientation(value string) bool { return value == "portrait" || value == "landscape" }

func templateSection(orientation string, doc IntroductionDocument) string {
	size := `<w:pgSz w:w="11906" w:h="16838"/>`
	if orientation == "landscape" {
		size = `<w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>`
	}
	margin := doc.pageMargins()
	offsets := ""
	if doc.Margins != nil || doc.MarginTwips != 0 {
		offsets = fmt.Sprintf(` w:header="%d" w:footer="%d"`, margin.Top/2, margin.Bottom/2)
	}
	return `<w:sectPr><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId3"/><w:type w:val="nextPage"/>` + size + fmt.Sprintf(`<w:pgMar w:top="%d" w:bottom="%d" w:left="%d" w:right="%d"%s/>`, margin.Top, margin.Bottom, margin.Left, margin.Right, offsets) + wordPageNumberFormat(doc) + `</w:sectPr>`
}

func renderTemplateBody(out *strings.Builder, doc IntroductionDocument, content reporting.ReportVersionContent, records []reporting.Record, types []reportType) error {
	selected := map[string]reportType{}
	for _, item := range types {
		selected[item.code] = item
	}
	teachers := headTeacherNames(content)
	orientation := doc.Orientation
	now := time.Now().In(time.FixedZone("UTC+8", 8*60*60))
	fields := map[string]string{"report.semesterName": content.Period.SemesterName, "report.weekNo": fmt.Sprint(content.Period.WeekNo), "report.weekNoChinese": chineseNumber(content.Period.WeekNo), "common.generatedDate": fmt.Sprintf("%d年%d月%d日", now.Year(), now.Month(), now.Day())}
	for _, node := range doc.Content {
		switch node.Type {
		case "sectionBreak":
			out.WriteString("<w:p><w:pPr>" + templateSection(orientation, doc) + "</w:pPr></w:p>")
			orientation = node.Attrs.Orientation
		case "pageBreak":
			out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		case "reportTable":
			item, ok := selected[node.Attrs.Key]
			if !ok {
				return ErrInvalidSelection
			}
			width := doc.textWidth(orientation)
			if item.code == "DORMITORY_HYGIENE" {
				dormitoryWordTables(out, content, records, item, teachers, width)
			} else {
				writeReportTable(out, content, records, item, teachers, "", width)
			}
		case "paragraph", "imprintTable", "wordTable":
			for i, run := range node.Content {
				if run.Type == "templateField" {
					value, ok := fields[run.Attrs.Key]
					if !ok || value == "" {
						return ErrInvalidSelection
					}
					node.Content[i] = IntroductionText{Type: "text", Text: value, Marks: run.Marks}
				}
			}
			if node.Type == "imprintTable" {
				writeImprintTable(out, node, doc.textWidth(orientation))
			} else if node.Type == "wordTable" {
				writeWordTable(out, node, *node.Attrs.Rows, node.Attrs.Border, doc.textWidth(orientation))
			} else {
				out.WriteString(renderIntroduction(IntroductionDocument{Content: []IntroductionParagraph{node}}))
			}
		}
	}
	out.WriteString(templateSection(orientation, doc))
	return nil
}

// writeImprintTable keeps the issuing office and date aligned even when either cell wraps.
func writeImprintTable(out *strings.Builder, node IntroductionParagraph, pageWidth int) {
	writeWordTable(out, node, [][]layoutCell{{{Length: node.Attrs.CellSplit, Align: "left"}, {Length: len(node.Content) - node.Attrs.CellSplit, Align: "right"}}}, "horizontalEdges", pageWidth)
}

func writeWordTable(out *strings.Builder, node IntroductionParagraph, rows [][]layoutCell, border string, pageWidth int) {
	tableWidth := 10000 - node.Attrs.InsetLeft - node.Attrs.InsetRight
	fmt.Fprintf(out, `<w:tbl><w:tblPr><w:tblW w:type="pct" w:w="%d"/><w:tblInd w:type="dxa" w:w="%d"/><w:tblBorders>`, tableWidth/2, pageWidth*node.Attrs.InsetLeft/10000)
	pageWidth = pageWidth * tableWidth / 10000
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		style := "nil"
		if border == "all" || (border == "horizontalEdges" && (edge == "top" || edge == "bottom")) {
			style = "single"
		}
		fmt.Fprintf(out, `<w:%s w:val="%s" w:sz="6"/>`, edge, style)
	}
	out.WriteString(`</w:tblBorders></w:tblPr><w:tblGrid>`)
	for _, c := range rows[0] {
		width := c.Width
		if width == 0 {
			width = 10000 / len(rows[0])
		}
		fmt.Fprintf(out, `<w:gridCol w:w="%d"/>`, pageWidth*width/10000)
	}
	out.WriteString("</w:tblGrid>")
	offset := 0
	for _, row := range rows {
		out.WriteString("<w:tr><w:trPr><w:cantSplit/>")
		if row[0].Height > 0 {
			fmt.Fprintf(out, `<w:trHeight w:val="%d" w:hRule="atLeast"/>`, row[0].Height*20)
		}
		out.WriteString("</w:trPr>")
		for _, cell := range row {
			width := cell.Width
			if width == 0 {
				width = 10000 / len(row)
			}
			fmt.Fprintf(out, `<w:tc><w:tcPr><w:tcW w:type="pct" w:w="%d"/>`, width/2)
			if cell.HiddenEdges != 0 {
				out.WriteString("<w:tcBorders>")
				for i, edge := range []string{"top", "right", "bottom", "left"} {
					if cell.HiddenEdges&(1<<i) != 0 {
						fmt.Fprintf(out, `<w:%s w:val="nil"/>`, edge)
					}
				}
				out.WriteString("</w:tcBorders>")
			}
			if cell.VerticalAlign != "" {
				fmt.Fprintf(out, `<w:vAlign w:val="%s"/>`, cell.VerticalAlign)
			}
			out.WriteString("</w:tcPr>")
			runs := node.Content[offset : offset+cell.Length]
			offset += cell.Length
			out.WriteString(renderIntroductionWithCellSize(IntroductionDocument{Content: []IntroductionParagraph{{Type: "paragraph", Attrs: IntroductionAttrs{Style: "body", TextAlign: cell.Align, LineSpacingMode: "single"}, Content: runs}}}, cell.FontSize))
			out.WriteString("</w:tc>")
		}
		out.WriteString("</w:tr>")
	}
	out.WriteString("</w:tbl>")
}

func (doc IntroductionDocument) textWidth(orientation string) int {
	width := 11906
	if orientation == "landscape" {
		width = 16838
	}
	m := doc.pageMargins()
	return width - m.Left - m.Right
}
