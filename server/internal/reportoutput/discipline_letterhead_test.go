package reportoutput

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestDisciplineLetterheadBundle(t *testing.T) {
	raw := []byte(`{"schemaVersion":2,"content":[{"type":"paragraph","attrs":{"style":"title","textAlign":"center","letterhead":true},"content":[{"type":"text","text":"示例学院"}]},{"type":"paragraph","attrs":{"style":"body","textAlign":"center"},"content":[{"type":"templateField","attrs":{"key":"discipline.issueNumber"}},{"type":"templateField","attrs":{"key":"discipline.authorityNumber"}},{"type":"templateField","attrs":{"key":"discipline.clauseReference"}}]},{"type":"paragraph","attrs":{"style":"body","textAlign":"left","letterhead":true,"border":"redRule"}}]}`)
	fields := map[string]string{"discipline.issueNumber": "示例〔2026〕1号", "discipline.authorityNumber": "示例规〔2025〕3号", "discipline.clauseReference": "第二章第三条"}
	now := time.Unix(0, 0)
	bundle, err := RenderDisciplineOutput(raw, fields, now, "both")
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil || len(z.File) != 2 {
		t.Fatal("invalid bundle", err)
	}
	for i, variant := range []string{"letterhead", "plain"} {
		data, err := RenderDisciplineOutput(raw, fields, now, variant)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"处分决定书（带红头版）.docx", "处分决定书（无红头版）.docx"}[i]; z.File[i].Name != want {
			t.Fatal(z.File[i].Name)
		}
		packaged, err := fs.ReadFile(z, z.File[i].Name)
		if err != nil || !bytes.Equal(packaged, data) {
			t.Fatal("single and bundled editions differ", err)
		}
		word, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		file, err := word.Open("word/document.xml")
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		xml := string(contents)
		if !strings.Contains(xml, fields["discipline.authorityNumber"]) || !strings.Contains(xml, fields["discipline.clauseReference"]) || !strings.Contains(xml, fields["discipline.issueNumber"]) || strings.Contains(xml, "示例学院") != (i == 0) || strings.Contains(xml, "FF0000") != (i == 0) || strings.Count(xml, "<w:p>") != 3 {
			t.Fatal("unexpected rendered rows", xml)
		}
	}
	if _, err := RenderDisciplineOutput(raw, nil, now, "both"); err == nil {
		t.Fatal("exported incomplete pair")
	}
}

func TestDisciplineDocumentBasisRequiredWithoutPlaceholders(t *testing.T) {
	document := []byte(`{"schemaVersion":2,"content":[{"type":"paragraph","content":[{"type":"text","text":"示例"}]}]}`)
	preview := DocumentPreview{Document: document, Fields: map[string]string{"discipline.authorityNumber": "测试文号", "discipline.clauseReference": " "}}
	if err := preview.Fill(DocumentRequest{}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if len(preview.Missing) != 1 || preview.Missing[0] != "discipline.clauseReference" {
		t.Fatalf("missing fields: %v", preview.Missing)
	}
	legacy := []byte(`{"schemaVersion":2,"content":[{"type":"paragraph","content":[{"type":"templateField","attrs":{"key":"discipline.clause"}}]}]}`)
	if err := ValidateDisciplineTemplate(legacy); err == nil {
		t.Fatal("retired clause accepted")
	}
}
