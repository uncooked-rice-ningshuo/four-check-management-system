package reportoutput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// IntroductionDocument is the small Tiptap subset accepted for a report cover.
// Unknown nodes and formatting are rejected before the document reaches OOXML.
type IntroductionDocument struct {
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

type IntroductionAttrs struct {
	Style           string `json:"style"`
	TextAlign       string `json:"textAlign"`
	Indent          int    `json:"indent,omitempty"`
	SpaceBefore     int    `json:"spaceBefore,omitempty"`
	SpaceAfter      int    `json:"spaceAfter,omitempty"`
	LineSpacing     int    `json:"lineSpacing,omitempty"`
	LineSpacingMode string `json:"lineSpacingMode,omitempty"`
}

type IntroductionText struct {
	Type  string             `json:"type"`
	Text  string             `json:"text"`
	Marks []IntroductionMark `json:"marks,omitempty"`
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
var allowedIntroductionSizes = map[string]bool{"14pt": true, "15pt": true, "16pt": true, "20pt": true, "30pt": true}
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
	if document.Type != "doc" || len(document.Content) == 0 || len(document.Content) > 200 {
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
	if document.FooterLayout == "custom" && (!utf8.ValidString(document.FooterText) || utf8.RuneCountInString(document.FooterText) > 240 || strings.Contains(document.FooterText, "\r") || strings.Count(document.FooterText, "\n") > 2 || strings.Count(document.FooterText, "{page}") > 1) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	if document.FooterLayout == "custom" {
		for _, line := range strings.Split(document.FooterText, "\n") {
			if strings.Count(line, "\t") > 1 {
				return IntroductionDocument{}, ErrInvalidSelection
			}
		}
	}
	if document.TitleTemplate != "" {
		check := strings.ReplaceAll(strings.ReplaceAll(document.TitleTemplate, "{semesterName}", ""), "{weekNo}", "")
		if !validPageText(document.TitleTemplate) || strings.ContainsAny(check, "{}") || document.TitleTemplate == " " {
			return IntroductionDocument{}, ErrInvalidSelection
		}
	}
	if document.AttachmentPosition != nil && (*document.AttachmentPosition < 0 || *document.AttachmentPosition > len(document.Content)) {
		return IntroductionDocument{}, ErrInvalidSelection
	}
	characters := 0
	for _, paragraph := range document.Content {
		if paragraph.Type != "paragraph" || !allowedIntroductionStyles[paragraph.Attrs.Style] || !allowedIntroductionAlignment[paragraph.Attrs.TextAlign] || paragraph.Attrs.Indent < 0 || paragraph.Attrs.Indent > 4 || paragraph.Attrs.SpaceBefore < 0 || paragraph.Attrs.SpaceBefore > 72 || paragraph.Attrs.SpaceAfter < 0 || paragraph.Attrs.SpaceAfter > 72 || paragraph.Attrs.LineSpacing < 0 || paragraph.Attrs.LineSpacing > 48 || (paragraph.Attrs.LineSpacing > 0 && paragraph.Attrs.LineSpacing < 18) || !allowedLineSpacingMode(paragraph.Attrs.LineSpacingMode) {
			return IntroductionDocument{}, ErrInvalidSelection
		}
		for _, run := range paragraph.Content {
			if run.Type != "text" || run.Text == "" || strings.ContainsAny(run.Text, "\r\n") || !utf8.ValidString(run.Text) {
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
