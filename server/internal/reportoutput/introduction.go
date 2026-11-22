package reportoutput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// IntroductionDocument is the small Tiptap subset accepted for a report cover.
// Unknown nodes and formatting are rejected before the document reaches OOXML.
type IntroductionDocument struct {
	SchemaVersion        int                     `json:"schemaVersion,omitempty"`
	Orientation          string                  `json:"orientation,omitempty"`
	Type                 string                  `json:"type"`
	TitleTemplate        string                  `json:"titleTemplate,omitempty"`
	AttachmentPosition   *int                    `json:"attachmentPosition,omitempty"`
	ManualAttachmentList bool                    `json:"manualAttachmentList,omitempty"`
	HeaderText           string                  `json:"headerText,omitempty"`
	FooterText           string                  `json:"footerText,omitempty"`
	FooterLayout         string                  `json:"footerLayout,omitempty"`
	FooterAlign          string                  `json:"footerAlign,omitempty"`
	Content              []IntroductionParagraph `json:"content"`
}

type IntroductionParagraph struct {
	Type    string             `json:"type"`
	Attrs   IntroductionAttrs  `json:"attrs"`
	Content []IntroductionText `json:"content,omitempty"`
}

type layoutCell struct {
	Width       int `json:"width,omitempty"`       // hundredths of the table width percentage
	Height      int `json:"height,omitempty"`      // minimum row height in points
	HiddenEdges int `json:"hiddenEdges,omitempty"` // top, right, bottom, left

	Length int    `json:"length"`
	Align  string `json:"align"`
}
type IntroductionAttrs struct {
	InsetLeft  int             `json:"insetLeft,omitempty"`
	InsetRight int             `json:"insetRight,omitempty"`
	Rows       *[][]layoutCell `json:"rows,omitempty"`

	CellSplit       int    `json:"cellSplit,omitempty"`
	Border          string `json:"border,omitempty"`
	Key             string `json:"key,omitempty"`
	Orientation     string `json:"orientation,omitempty"`
	Style           string `json:"style"`
	TextAlign       string `json:"textAlign"`
	Indent          int    `json:"indent,omitempty"`
	SpaceBefore     int    `json:"spaceBefore,omitempty"`
	SpaceAfter      int    `json:"spaceAfter,omitempty"`
	LineSpacing     int    `json:"lineSpacing,omitempty"`
	LineSpacingMode string `json:"lineSpacingMode,omitempty"`
}

type IntroductionText struct {
	Attrs *TemplateFieldAttrs `json:"attrs,omitempty"`
	Type  string              `json:"type"`
	Text  string              `json:"text"`
	Marks []IntroductionMark  `json:"marks,omitempty"`
}

type TemplateFieldAttrs struct {
	Key string `json:"key"`
}

type IntroductionMark struct {
	Type  string            `json:"type"`
	Attrs *IntroductionFont `json:"attrs,omitempty"`
}

type IntroductionFont struct {
	FontFamily string `json:"fontFamily,omitempty"`
	FontSize   string `json:"fontSize,omitempty"`
}

var allowedIntroductionFonts = map[string]bool{"仿宋": true, "宋体": true, "黑体": true, "楷体": true, "微软雅黑": true, "华文新魏": true}
var allowedIntroductionSizes = map[string]bool{"9pt": true, "10.5pt": true, "12pt": true, "14pt": true, "15pt": true, "16pt": true, "20pt": true, "30pt": true}
var allowedIntroductionStyles = map[string]bool{"body": true, "title": true, "subtitle": true, "heading": true, "heading1": true, "heading2": true, "heading3": true, "signature": true, "date": true}
var allowedIntroductionAlignment = map[string]bool{"left": true, "center": true, "right": true, "justify": true}

func allowedLineSpacingMode(mode string) bool {
	switch mode {
	case "", "single", "oneHalf", "double", "fixed":
		return true
	default:
		return false
	}
}

func decodeIntroduction(raw json.RawMessage) (IntroductionDocument, error) {
	return decodeTemplate(raw, false)
}

func decodeTemplate(raw json.RawMessage, discipline bool) (IntroductionDocument, error) {
	if len(raw) == 0 || len(raw) > 65536 {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	var document IntroductionDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return IntroductionDocument{}, fmt.Errorf("invalid introduction document: %w", ErrInvalidSelection)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if discipline && document.SchemaVersion != 2 {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.SchemaVersion != 0 && document.SchemaVersion != 2 || (document.Orientation != "" && (document.SchemaVersion != 2 || !validOrientation(document.Orientation))) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.Type != "doc" && !(document.SchemaVersion == 2 && document.Type == "") || len(document.Content) == 0 || len(document.Content) > 200 {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if !validPageText(document.HeaderText) || (document.FooterLayout != "custom" && !validPageText(document.FooterText)) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	switch document.FooterLayout {
	case "", "stacked", "inline", "textOnly", "numberOnly", "custom":
	default:
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.FooterAlign != "" && document.FooterAlign != "left" && document.FooterAlign != "center" && document.FooterAlign != "right" {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.FooterLayout == "custom" && (!utf8.ValidString(document.FooterText) || utf8.RuneCountInString(document.FooterText) > 240 || strings.Contains(document.FooterText, "\r") || strings.Count(document.FooterText, "\n") > 2 || strings.Count(document.FooterText, "{page}")+strings.Count(document.FooterText, "{pageChinese}") > 1) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.FooterLayout == "custom" {
		plain := strings.NewReplacer("{page}", "", "{pageChinese}", "", "{totalPages}", "", "{totalPagesChinese}", "", "{semesterName}", "", "{weekNo}", "", "{weekNoChinese}", "").Replace(document.FooterText)
		if strings.ContainsAny(plain, "{}") || strings.Count(document.FooterText, "{totalPages}")+strings.Count(document.FooterText, "{totalPagesChinese}") > 1 {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		for _, line := range strings.Split(document.FooterText, "\n") {
			if strings.Count(line, "\t") > 2 {
				return IntroductionDocument{}, ErrInvalidSelection
			}
		}
	}
	if document.TitleTemplate != "" {
		check := strings.NewReplacer("{semesterName}", "", "{weekNo}", "", "{weekNoChinese}", "", "{reportDate}", "").Replace(document.TitleTemplate)
		if !validPageText(document.TitleTemplate) || strings.ContainsAny(check, "{}") || document.TitleTemplate == " " {
			return IntroductionDocument{}, ErrInvalidSelection
		}
	}
	if document.AttachmentPosition != nil && (*document.AttachmentPosition < 0 || *document.AttachmentPosition > len(document.Content)) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	characters := 0
	for idx := range document.Content {
		paragraph := &document.Content[idx]
		isImprint := document.SchemaVersion == 2 && paragraph.Type == "imprintTable"
		isTable := document.SchemaVersion == 2 && paragraph.Type == "wordTable"
		if isTable {
			if !validWordTable(*paragraph) {
				return IntroductionDocument{}, ErrInvalidSelection
			}
		} else if paragraph.Attrs.Rows != nil || paragraph.Attrs.InsetLeft != 0 || paragraph.Attrs.InsetRight != 0 {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		if isImprint {
			if paragraph.Attrs.CellSplit < 0 || paragraph.Attrs.CellSplit > len(paragraph.Content) || paragraph.Attrs != (IntroductionAttrs{CellSplit: paragraph.Attrs.CellSplit}) {
				return IntroductionDocument{}, ErrInvalidSelection
			}
		} else if paragraph.Attrs.CellSplit != 0 {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		if document.SchemaVersion == 2 && paragraph.Type == "paragraph" {
			if paragraph.Attrs.Style == "" {
				paragraph.Attrs.Style = "body"
			}
			if paragraph.Attrs.TextAlign == "" {
				paragraph.Attrs.TextAlign = "left"
			}
		}
		if document.SchemaVersion == 2 && paragraph.Type != "paragraph" && !isImprint && !isTable {
			if len(paragraph.Content) != 0 {
				return IntroductionDocument{}, ErrInvalidSelection
			}
			switch paragraph.Type {
			case "sectionBreak":
				if !validOrientation(paragraph.Attrs.Orientation) || paragraph.Attrs != (IntroductionAttrs{Orientation: paragraph.Attrs.Orientation}) {
					return IntroductionDocument{}, ErrInvalidSelection
				}
			case "pageBreak":
				if paragraph.Attrs != (IntroductionAttrs{}) {
					return IntroductionDocument{}, ErrInvalidSelection
				}
			case "reportTable":
				if discipline {
					return IntroductionDocument{}, ErrInvalidSelection
				}
				if paragraph.Attrs != (IntroductionAttrs{Key: paragraph.Attrs.Key}) || paragraph.Attrs.Key == "" {
					return IntroductionDocument{}, ErrInvalidSelection
				}
				if _, err := chosenTypes([]string{paragraph.Attrs.Key}); err != nil {
					return IntroductionDocument{}, err
				}
			default:
				return IntroductionDocument{}, ErrInvalidSelection
			}
			continue
		}
		if paragraph.Attrs.Key != "" || paragraph.Attrs.Orientation != "" {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		if !isTable {
			switch paragraph.Attrs.Border {
			case "", "topBottom", "bottomDouble":
			default:
				return IntroductionDocument{}, ErrInvalidSelection
			}
		}
		if !isTable && !isImprint && (paragraph.Type != "paragraph" || !allowedIntroductionStyles[paragraph.Attrs.Style] || !allowedIntroductionAlignment[paragraph.Attrs.TextAlign] || paragraph.Attrs.Indent < 0 || paragraph.Attrs.Indent > 4 || paragraph.Attrs.SpaceBefore < 0 || paragraph.Attrs.SpaceBefore > 72 || paragraph.Attrs.SpaceAfter < 0 || paragraph.Attrs.SpaceAfter > 72 || paragraph.Attrs.LineSpacing < 0 || paragraph.Attrs.LineSpacing > 48 || (paragraph.Attrs.LineSpacing > 0 && paragraph.Attrs.LineSpacing < 18) || !allowedLineSpacingMode(paragraph.Attrs.LineSpacingMode)) {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		for _, run := range paragraph.Content {
			if run.Type == "templateField" && document.SchemaVersion == 2 {
				if run.Attrs == nil || run.Text != "" || !allowedTemplateField(run.Attrs.Key, discipline) {
					return IntroductionDocument{}, ErrInvalidSelection
				}
			} else if run.Attrs != nil || run.Type != "text" || run.Text == "" || strings.ContainsAny(run.Text, "\r\n") || !utf8.ValidString(run.Text) {
				return IntroductionDocument{}, ErrInvalidSelection
			}
			characters += utf8.RuneCountInString(run.Text)
			if characters > 20000 {
				return IntroductionDocument{}, ErrInvalidSelection
			}
			seen := map[string]bool{}
			for _, mark := range run.Marks {
				if seen[mark.Type] {
					return IntroductionDocument{}, ErrInvalidSelection
				}
				seen[mark.Type] = true
				switch mark.Type {
				case "bold", "italic", "strike", "underline":
					if mark.Attrs != nil {
						return IntroductionDocument{}, ErrInvalidSelection
					}
				case "textStyle":
					if mark.Attrs == nil || (mark.Attrs.FontFamily == "" && mark.Attrs.FontSize == "") ||
						(mark.Attrs.FontFamily != "" && !allowedIntroductionFonts[mark.Attrs.FontFamily]) ||
						(mark.Attrs.FontSize != "" && !allowedIntroductionSizes[mark.Attrs.FontSize]) {
						return IntroductionDocument{}, ErrInvalidSelection
					}
				default:
					return IntroductionDocument{}, ErrInvalidSelection
				}
			}
		}
	}
	return document, nil
}

func validPageText(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 120 && !strings.ContainsAny(value, "\r\n")
}

// resolveIntroductionDate freezes the operator-selected issuing date in each
// generated file, independently of the report data version and layout setting.
func resolveIntroductionDate(raw json.RawMessage, issueDate string) (json.RawMessage, string, error) {
	document, err := decodeIntroduction(raw)
	if err != nil {
		return nil, "", err
	}
	required := strings.Contains(document.TitleTemplate, "{reportDate}")
	for _, paragraph := range document.Content {
		for _, run := range paragraph.Content {
			required = required || (run.Attrs != nil && run.Attrs.Key == "report.reportDate") || (document.SchemaVersion == 0 && strings.Contains(run.Text, "{reportDate}"))
		}
	}
	if !required {
		return raw, "", nil
	}
	date, err := time.Parse("2006-01-02", issueDate)
	if err != nil || date.Format("2006-01-02") != issueDate {
		return nil, "", ErrInvalidSelection
	}
	formatted := fmt.Sprintf("%d年%d月%d日", date.Year(), date.Month(), date.Day())
	document.TitleTemplate = strings.ReplaceAll(document.TitleTemplate, "{reportDate}", formatted)
	for index := range document.Content {
		for run := range document.Content[index].Content {
			r := &document.Content[index].Content[run]
			if r.Attrs != nil && r.Attrs.Key == "report.reportDate" {
				*r = IntroductionText{Type: "text", Text: formatted, Marks: r.Marks}
			}
			if document.SchemaVersion == 0 {
				r.Text = strings.ReplaceAll(r.Text, "{reportDate}", formatted)
			}
		}
	}
	resolved, err := json.Marshal(document)
	if err != nil {
		return nil, "", err
	}
	return resolved, issueDate, nil
}

func legacyIntroduction(value string) json.RawMessage {
	paragraphs := make([]IntroductionParagraph, 0)
	for _, line := range strings.Split(value, "\n") {
		paragraph := IntroductionParagraph{Type: "paragraph", Attrs: IntroductionAttrs{Style: "body", TextAlign: "left"}}
		if line != "" {
			paragraph.Content = []IntroductionText{{Type: "text", Text: line}}
		}
		paragraphs = append(paragraphs, paragraph)
	}
	raw, _ := json.Marshal(IntroductionDocument{Type: "doc", Content: paragraphs})
	return raw
}

func plainIntroduction(document IntroductionDocument) string {
	lines := make([]string, 0, len(document.Content))
	for _, paragraph := range document.Content {
		var line strings.Builder
		for _, run := range paragraph.Content {
			line.WriteString(run.Text)
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

func allowedTemplateField(key string, discipline bool) bool {
	switch key {
	case "common.generatedDate":
		return true
	case "report.semesterName", "report.weekNo", "report.weekNoChinese", "report.reportDate":
		return !discipline
	}
	if discipline {
		for _, field := range DisciplineFields {
			if key == "discipline."+field {
				return true
			}
		}
	}
	return false
}

// ValidateDisciplineTemplate checks the discipline field catalog before persisting a revision.
func ValidateDisciplineTemplate(raw json.RawMessage) error {
	_, err := decodeTemplate(raw, true)
	return err
}

func validWordTable(node IntroductionParagraph) bool {
	a := node.Attrs
	if a.InsetLeft < 0 || a.InsetLeft > 9000 || a.InsetRight < 0 || a.InsetRight > 9000-a.InsetLeft || a.Rows == nil || a != (IntroductionAttrs{Rows: a.Rows, Border: a.Border, InsetLeft: a.InsetLeft, InsetRight: a.InsetRight}) || (a.Border != "all" && a.Border != "none" && a.Border != "horizontalEdges") {
		return false
	}
	rows := *a.Rows
	if len(rows) == 0 || len(rows) > 200 || len(rows[0]) == 0 || len(rows[0]) > 32 {
		return false
	}
	count := 0
	for _, row := range rows {
		if len(row) != len(rows[0]) {
			return false
		}
		for _, c := range row {
			if c.Width < 0 || c.Width > 10000 || c.Height < 0 || c.Height > 720 || c.HiddenEdges < 0 || c.HiddenEdges > 15 {
				return false
			}
			if (c.Align != "left" && c.Align != "right" && c.Align != "center") || c.Length < 0 || c.Length > len(node.Content)-count {
				return false
			}
			count += c.Length
		}
	}
	return count == len(node.Content)
}
