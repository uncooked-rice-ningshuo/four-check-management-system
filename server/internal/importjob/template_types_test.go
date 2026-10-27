package importjob

import "testing"

// Every advertised type must stay publishable: a code without a template
// definition or a workbook builder would leave its download permanently broken.
func TestPublishedTemplateTypeCodesAreBuildable(t *testing.T) {
	codes := PublishedTemplateTypeCodes()
	if len(codes) == 0 {
		t.Fatal("published template type codes must not be empty")
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate template type code %s", code)
		}
		seen[code] = true
		definition, err := InspectionTemplate(code)
		if err != nil {
			t.Fatalf("type %s has no template definition: %v", code, err)
		}
		content, err := BuildInspectionTemplate(code)
		if err != nil || len(content) == 0 {
			t.Fatalf("type %s template cannot be built: %v", code, err)
		}
		parser, err := NewInspectionXLSXParser(code, 16<<20)
		if err != nil {
			t.Fatalf("type %s has no parser: %v", code, err)
		}
		if _, err := parser.Parse(definition.Filename, content); err != nil {
			t.Fatalf("type %s template fails its own parser: %v", code, err)
		}
	}
	for _, required := range []string{"CLASSROOM_HYGIENE", "EVENING_SELF_STUDY"} {
		if !seen[required] {
			t.Fatalf("missing template type %s", required)
		}
	}
}
