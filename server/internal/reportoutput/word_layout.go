package reportoutput

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const wordStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="SimSun" w:eastAsia="宋体"/><w:sz w:val="20"/></w:rPr></w:rPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="CoverTitle"><w:name w:val="通报标题"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="0" w:after="0"/></w:pPr><w:rPr><w:rFonts w:ascii="STXinwei" w:eastAsia="华文新魏"/><w:sz w:val="60"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverHeading"><w:name w:val="说明小标题"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="240" w:after="120" w:line="420" w:lineRule="exact"/><w:keepNext/></w:pPr><w:rPr><w:rFonts w:ascii="SimHei" w:eastAsia="黑体"/><w:sz w:val="40"/><w:b/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverSubtitle"><w:name w:val="通报副标题"/><w:basedOn w:val="CoverTitle"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="160" w:after="0"/></w:pPr><w:rPr><w:rFonts w:ascii="SimHei" w:eastAsia="黑体"/><w:sz w:val="40"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverHeading1"><w:name w:val="一级标题"/><w:basedOn w:val="CoverHeading"/></w:style>
<w:style w:type="paragraph" w:styleId="CoverHeading2"><w:name w:val="二级标题"/><w:basedOn w:val="CoverHeading"/><w:rPr><w:sz w:val="32"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverHeading3"><w:name w:val="三级标题"/><w:basedOn w:val="CoverHeading"/><w:rPr><w:sz w:val="28"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverBody"><w:name w:val="通报正文"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0" w:line="540" w:lineRule="exact"/></w:pPr><w:rPr><w:rFonts w:ascii="FangSong" w:eastAsia="仿宋"/><w:sz w:val="30"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverSignature"><w:name w:val="落款"/><w:basedOn w:val="CoverBody"/><w:pPr><w:jc w:val="right"/><w:ind w:firstLineChars="0"/><w:spacing w:before="260"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="CoverDate"><w:name w:val="日期"/><w:basedOn w:val="CoverSignature"/></w:style>
	<w:style w:type="paragraph" w:styleId="CoverAttachment"><w:name w:val="附件说明"/><w:basedOn w:val="CoverBody"/><w:pPr><w:ind w:firstLineChars="200"/><w:spacing w:before="0" w:after="0" w:line="540" w:lineRule="exact"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="AttachmentTitle"><w:name w:val="附件标题"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:after="180"/></w:pPr><w:rPr><w:rFonts w:ascii="SimHei" w:eastAsia="黑体"/><w:sz w:val="32"/><w:b/></w:rPr></w:style>
</w:styles>`

func coverTitle(value string) string {
	return `<w:p><w:pPr><w:pStyle w:val="CoverTitle"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p>`
}

func attachmentTitle(value string) string {
	return `<w:p><w:pPr><w:pStyle w:val="AttachmentTitle"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p>`
}

func coverAttachmentList(types []reportType) string {
	var output strings.Builder
	for index, item := range types {
		label := fmt.Sprintf("附件%d：%s", index+1, item.title)
		output.WriteString(`<w:p><w:pPr><w:pStyle w:val="CoverAttachment"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(label) + `</w:t></w:r></w:p>`)
	}
	return output.String()
}

func wordHeaderXML(value string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p></w:hdr>`
}

func wordFooterXML(value string) string {
	return wordFooterLayoutXML(value, "stacked")
}

func wordFooterLayoutXML(value, layout string) string {
	const open = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`
	const close = `</w:ftr>`
	text := `<w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r>`
	page := `<w:fldSimple w:instr="PAGE"/>`
	paragraph := func(align, content string) string {
		return `<w:p><w:pPr><w:jc w:val="` + align + `"/><w:spacing w:before="0" w:after="0"/></w:pPr>` + content + `</w:p>`
	}
	switch layout {
	case "textOnly":
		return open + paragraph("left", text) + close
	case "numberOnly":
		return open + paragraph("right", page) + close
	case "inline":
		// A two-cell table keeps text and the PAGE field on one line on both
		// portrait and landscape sections without a page-width-specific tab stop.
		cell := func(align, content string) string {
			return `<w:tc><w:tcPr><w:tcW w:w="2500" w:type="pct"/></w:tcPr>` + paragraph(align, content) + `</w:tc>`
		}
		return open + `<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders><w:top w:val="nil"/><w:left w:val="nil"/><w:bottom w:val="nil"/><w:right w:val="nil"/><w:insideH w:val="nil"/><w:insideV w:val="nil"/></w:tblBorders></w:tblPr><w:tblGrid><w:gridCol w:w="4500"/><w:gridCol w:w="4500"/></w:tblGrid><w:tr>` + cell("left", text) + cell("right", page) + `</w:tr></w:tbl>` + paragraph("left", "") + close
	default:
		return open + paragraph("center", text) + paragraph("center", page) + close
	}
}

func wordFooterDocumentXML(document IntroductionDocument) string {
	if document.FooterLayout != "custom" {
		return wordFooterLayoutXML(document.FooterText, document.FooterLayout)
	}
	const open = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`
	runs := func(value string) string {
		parts := strings.Split(value, "{page}")
		var output strings.Builder
		for index, part := range parts {
			if part != "" {
				output.WriteString(`<w:r><w:t xml:space="preserve">` + xmlText(part) + `</w:t></w:r>`)
			}
			if index < len(parts)-1 {
				output.WriteString(`<w:fldSimple w:instr="PAGE"/>`)
			}
		}
		return output.String()
	}
	paragraph := func(align, content string) string {
		return `<w:p><w:pPr><w:jc w:val="` + align + `"/><w:spacing w:before="0" w:after="0"/></w:pPr>` + content + `</w:p>`
	}
	align := document.FooterAlign
	if align == "" {
		align = "center"
	}
	var output strings.Builder
	output.WriteString(open)
	for _, line := range strings.Split(document.FooterText, "\n") {
		if left, right, ok := strings.Cut(line, "\t"); ok {
			cell := func(cellAlign, content string) string {
				return `<w:tc><w:tcPr><w:tcW w:w="2500" w:type="pct"/></w:tcPr>` + paragraph(cellAlign, content) + `</w:tc>`
			}
			output.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders><w:top w:val="nil"/><w:left w:val="nil"/><w:bottom w:val="nil"/><w:right w:val="nil"/><w:insideH w:val="nil"/><w:insideV w:val="nil"/></w:tblBorders></w:tblPr><w:tblGrid><w:gridCol w:w="4500"/><w:gridCol w:w="4500"/></w:tblGrid><w:tr>` + cell("left", runs(left)) + cell("right", runs(right)) + `</w:tr></w:tbl>`)
		} else {
			output.WriteString(paragraph(align, runs(line)))
		}
	}
	output.WriteString(`</w:ftr>`)
	return output.String()
}

func renderIntroduction(document IntroductionDocument) string {
	styles := map[string]string{"body": "CoverBody", "title": "CoverTitle", "subtitle": "CoverSubtitle", "heading": "CoverHeading", "heading1": "CoverHeading1", "heading2": "CoverHeading2", "heading3": "CoverHeading3", "signature": "CoverSignature", "date": "CoverDate"}
	alignments := map[string]string{"left": "left", "center": "center", "right": "right", "justify": "both"}
	sizes := map[string]string{"14pt": "28", "15pt": "30", "16pt": "32", "20pt": "40", "30pt": "60"}
	var output strings.Builder
	for _, paragraph := range document.Content {
		output.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + styles[paragraph.Attrs.Style] + `"/><w:jc w:val="` + alignments[paragraph.Attrs.TextAlign] + `"/>`)
		if paragraph.Attrs.Indent > 0 {
			output.WriteString(fmt.Sprintf(`<w:ind w:firstLineChars="%d"/>`, paragraph.Attrs.Indent*100))
		}
		if paragraph.Attrs.SpaceBefore > 0 || paragraph.Attrs.SpaceAfter > 0 || paragraph.Attrs.LineSpacing > 0 || paragraph.Attrs.LineSpacingMode != "" {
			spacing := fmt.Sprintf(`<w:spacing w:before="%d" w:after="%d"`, paragraph.Attrs.SpaceBefore*20, paragraph.Attrs.SpaceAfter*20)
			switch paragraph.Attrs.LineSpacingMode {
			case "single":
				spacing += ` w:line="240" w:lineRule="auto"`
			case "oneHalf":
				spacing += ` w:line="360" w:lineRule="auto"`
			case "double":
				spacing += ` w:line="480" w:lineRule="auto"`
			case "fixed", "":
				if paragraph.Attrs.LineSpacing > 0 {
					spacing += fmt.Sprintf(` w:line="%d" w:lineRule="exact"`, paragraph.Attrs.LineSpacing*20)
				}
			}
			output.WriteString(spacing + `/>`)
		}
		output.WriteString(`</w:pPr>`)
		for _, run := range paragraph.Content {
			output.WriteString(`<w:r><w:rPr>`)
			for _, mark := range run.Marks {
				if mark.Type == "bold" {
					output.WriteString(`<w:b/>`)
				}
				if mark.Type == "italic" {
					output.WriteString(`<w:i/>`)
				}
				if mark.Type == "strike" {
					output.WriteString(`<w:strike/>`)
				}
				if mark.Type == "underline" {
					output.WriteString(`<w:u w:val="single"/>`)
				}
				if mark.Type == "textStyle" && mark.Attrs != nil {
					if mark.Attrs.FontFamily != "" {
						output.WriteString(`<w:rFonts w:ascii="` + xmlText(mark.Attrs.FontFamily) + `" w:eastAsia="` + xmlText(mark.Attrs.FontFamily) + `"/>`)
					}
					if mark.Attrs.FontSize != "" {
						output.WriteString(`<w:sz w:val="` + sizes[mark.Attrs.FontSize] + `"/>`)
					}
				}
			}
			output.WriteString(`</w:rPr><w:t xml:space="preserve">` + xmlText(run.Text) + `</w:t></w:r>`)
		}
		output.WriteString(`</w:p>`)
	}
	return output.String()
}

func wordColumnWidths(columns []column) []int {
	weights := make([]int, len(columns))
	total := 0
	for index, item := range columns {
		weight := 12
		switch item.title {
		case "备注", "原因":
			weight = 36
		case "情况", "责任班级", "责任寝室", "负责学生":
			weight = 24
		case "班级", "课程", "教室", "寝室":
			weight = 20
		case "学号", "日期":
			weight = 16
		}
		weights[index] = weight
		total += weight
	}
	widths := make([]int, len(columns))
	remaining := 15000
	for index, weight := range weights {
		if index == len(weights)-1 {
			widths[index] = remaining
		} else {
			widths[index] = 15000 * weight / total
			remaining -= widths[index]
		}
	}
	return widths
}

func estimatedRowHeight(values []string, widths []int) int {
	maxLines := 1
	for index, value := range values {
		charsPerLine := (widths[index] - 160) / 220
		if charsPerLine < 3 {
			charsPerLine = 3
		}
		lines := (utf8.RuneCountInString(value) + charsPerLine - 1) / charsPerLine
		if lines > maxLines {
			maxLines = lines
		}
	}
	return 360 + (maxLines-1)*280
}

// Hard page boundaries keep grade merges from continuing into a new page.
// This deliberately leaves room for each application's own line breaking.
func paginateWordRows(lines [][]string, widths []int) [][][]string {
	const pageBudget = 6500
	pages := make([][][]string, 0, 1)
	page := make([][]string, 0)
	used := 0
	for _, line := range lines {
		height := estimatedRowHeight(line, widths)
		if len(page) > 0 && used+height > pageBudget {
			pages = append(pages, page)
			page = make([][]string, 0)
			used = 0
		}
		page = append(page, line)
		used += height
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	return pages
}
