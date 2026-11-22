package reportoutput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
	"github.com/google/uuid"
)

// DisciplineTemplate lists published groups including those awaiting their first template.
type DisciplineTemplate struct {
	ViolationName    string          `json:"violationName,omitempty"`
	Unit             string          `json:"unit,omitempty"`
	ExampleAmount    string          `json:"exampleAmount,omitempty"`
	RuleID           string          `json:"ruleId"`
	GroupKey         string          `json:"groupKey"`
	TierCode         string          `json:"tierCode"`
	TierName         string          `json:"tierName"`
	TemplateID       string          `json:"templateId,omitempty"`
	SourceTemplateID string          `json:"sourceTemplateId,omitempty"`
	Name             string          `json:"name"`
	Revision         int             `json:"revision"`
	Document         json.RawMessage `json:"document,omitempty"`
}

// DocumentRequest fixes the edition and issuance inputs for preview and download.
type DocumentRequest struct {
	TemplateID   string `json:"templateId"`
	DecisionDate string `json:"decisionDate"`
	IssueDate    string `json:"issueDate"`
	IssueNumber  string `json:"issueNumber"`
}
type DocumentEdition struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}
type DocumentPreview struct {
	Document   json.RawMessage   `json:"document"`
	TemplateID string            `json:"templateId"`
	Editions   []DocumentEdition `json:"editions"`
	Fields     map[string]string `json:"fields"`
	Missing    []string          `json:"missing"`
}

var DisciplineFields = []string{"studentName", "studentNumber", "class", "group", "amount", "unit", "tier", "clause", "decidedAt", "decisionDate", "issuedDate", "issueNumber", "sex", "enrollmentYear", "major", "classNumber", "schoolYear", "semester", "violationCount", "violationName", "authorityNumber", "clauseReference"}

func (p *DocumentPreview) Fill(input DocumentRequest, now time.Time) error {
	p.Fields["common.generatedDate"] = now.In(time.FixedZone("UTC+8", 28800)).Format("2006年1月2日")
	// TODO(document-number): approved numbering and duplicate registration rules are pending.
	p.Fields["discipline.issueNumber"] = strings.TrimSpace(input.IssueNumber)
	for key, value := range map[string]string{"decisionDate": input.DecisionDate, "issuedDate": input.IssueDate} {
		if value == "" {
			continue
		}
		date, err := time.Parse("2006-01-02", value)
		if err != nil {
			return ErrInvalidWeekDate
		}
		formatted := date.Format("2006年1月2日")
		if key == "decisionDate" {
			var year strings.Builder
			for _, digit := range date.Format("2006") {
				year.WriteRune([]rune("○一二三四五六七八九")[digit-'0'])
			}
			formatted = year.String() + "年" + chineseNumber(int(date.Month())) + "月" + chineseNumber(date.Day()) + "日"
		}
		p.Fields["discipline."+key] = formatted
	}
	doc, err := decodeTemplate(p.Document, true)
	if err != nil {
		return err
	}
	p.Missing = []string{}
	seen := map[string]bool{}
	for _, node := range doc.Content {
		for _, r := range node.Content {
			if r.Type == "templateField" && strings.TrimSpace(p.Fields[r.Attrs.Key]) == "" && !seen[r.Attrs.Key] {
				p.Missing = append(p.Missing, r.Attrs.Key)
				seen[r.Attrs.Key] = true
			}
		}
	}
	return nil
}

type disciplineDocumentRepository interface {
	ListDocumentTemplates(context.Context) ([]DisciplineTemplate, error)
	SaveDocumentTemplate(context.Context, DisciplineTemplate, audit.Metadata) error
	ProcessDisciplineDocument(context.Context, string, DocumentRequest, audit.Metadata, bool) (DocumentPreview, []byte, error)
}

func (s *Service) documentRepository() (disciplineDocumentRepository, error) {
	repo, ok := s.repository.(disciplineDocumentRepository)
	if !ok {
		return nil, errors.New("discipline document repository unavailable")
	}
	return repo, nil
}
func (s *Service) DocumentTemplates(ctx context.Context) ([]DisciplineTemplate, error) {
	repo, err := s.documentRepository()
	if err != nil {
		return nil, err
	}
	return repo.ListDocumentTemplates(ctx)
}
func (s *Service) SaveDocumentTemplate(ctx context.Context, t DisciplineTemplate, meta audit.Metadata) error {
	if _, err := uuid.Parse(t.RuleID); err != nil || t.TierCode == "" || t.GroupKey == "" || t.Revision < 0 {
		return ErrInvalidSelection
	}
	if t.SourceTemplateID != "" {
		if _, err := uuid.Parse(t.SourceTemplateID); err != nil {
			return ErrInvalidSelection
		}
	}
	if err := ValidateDisciplineTemplate(t.Document); err != nil {
		return err
	}
	repo, err := s.documentRepository()
	if err != nil {
		return err
	}
	return repo.SaveDocumentTemplate(ctx, t, meta)
}
func (s *Service) DisciplineDocument(ctx context.Context, id string, input DocumentRequest, meta audit.Metadata, export bool) (DocumentPreview, []byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return DocumentPreview{}, nil, ErrInvalidSelection
	}
	if input.TemplateID != "" {
		if _, err := uuid.Parse(input.TemplateID); err != nil {
			return DocumentPreview{}, nil, ErrInvalidSelection
		}
	}
	if len([]rune(input.IssueNumber)) > 120 {
		return DocumentPreview{}, nil, ErrInvalidSelection
	}
	repo, err := s.documentRepository()
	if err != nil {
		return DocumentPreview{}, nil, err
	}
	return repo.ProcessDisciplineDocument(ctx, id, input, meta, export)
}

// RenderDisciplineDocument resolves fields exclusively against the selected case.
func RenderDisciplineDocument(raw json.RawMessage, fields map[string]string, now time.Time) ([]byte, error) {
	doc, err := decodeTemplate(raw, true)
	if err != nil {
		return nil, err
	}
	for i, p := range doc.Content {
		for j, r := range p.Content {
			if r.Type != "templateField" {
				continue
			}
			value := fields[r.Attrs.Key]
			if r.Attrs.Key == "common.generatedDate" {
				value = now.In(time.FixedZone("UTC+8", 28800)).Format("2006年1月2日")
			}
			if strings.TrimSpace(value) == "" {
				return nil, ErrInvalidSelection
			}
			doc.Content[i].Content[j] = IntroductionText{Type: "text", Text: value, Marks: r.Marks}
		}
	}
	var document strings.Builder
	document.WriteString(wordDocumentStart)
	if err := renderTemplateBody(&document, doc, reporting.ReportVersionContent{}, nil, nil); err != nil {
		return nil, err
	}
	document.WriteString("</w:body></w:document>")
	return packageWordDocument(document.String(), doc, "", 0)
}

var ErrTemplateUnchanged = errors.New("source document requires a substantive edit")

// CheckTemplateAdaptation compares the printable content, with unique values for
// each field. Editor defaults and JSON serialization alone cannot enable a copy.
func CheckTemplateAdaptation(source, target json.RawMessage) error {
	fields := map[string]string{}
	for _, key := range DisciplineFields {
		fields["discipline."+key] = "⟦discipline." + key + "⟧"
	}
	epoch := time.Unix(0, 0)
	a, err := RenderDisciplineDocument(source, fields, epoch)
	if err != nil {
		return err
	}
	b, err := RenderDisciplineDocument(target, fields, epoch)
	if err != nil {
		return err
	}
	if bytes.Equal(a, b) {
		return ErrTemplateUnchanged
	}
	return nil
}
