package inspection

import (
	"reflect"
	"testing"
)

func TestInspectionCatalog(t *testing.T) {
	definitions, err := InspectionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 7 {
		t.Fatalf("types=%d", len(definitions))
	}
	tables := map[string]bool{}
	for _, d := range definitions {
		if d.Code == "" || d.Name == "" || d.ExtensionTable == "" || d.TemplateVersion != 1 || len(d.Fields) == 0 || len(d.DuplicateKey) == 0 {
			t.Fatalf("incomplete %s", d.Code)
		}
		tables[d.ExtensionTable] = true
		seen := map[string]bool{}
		for _, f := range d.Fields {
			if f.Key == "" || f.Label == "" || f.Kind == "" || seen[f.Key] {
				t.Fatalf("invalid field %s/%s", d.Code, f.Key)
			}
			seen[f.Key] = true
		}

	}
	if len(tables) != 6 {
		t.Fatalf("tables=%d", len(tables))
	}
	l, _ := LookupInspectionDefinition("LATE_RETURN")
	if !reflect.DeepEqual(l.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"}) || l.TemplateStatus != "ACTIVE" {
		t.Fatal("late return import capability missing")
	}
	a, _ := LookupInspectionDefinition("ATTENDANCE_RATE")
	u, _ := LookupInspectionDefinition("UNATTENDED_CLASSROOM")
	if !reflect.DeepEqual(u.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"}) || u.TemplateStatus != "ACTIVE" {
		t.Fatal("unattended import capability missing")
	}
	if a.ExtensionTable != u.ExtensionTable || !reflect.DeepEqual(a.Fields, u.Fields) {
		t.Fatal("course absence schemas drifted")
	}
	c, _ := LookupInspectionDefinition("CLASSROOM_HYGIENE")
	if !reflect.DeepEqual(c.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"}) || c.TemplateStatus != "ACTIVE" {
		t.Fatal("classroom import capability missing")
	}
	m, _ := LookupInspectionDefinition("MORNING_ATTENDANCE")
	h, _ := LookupInspectionDefinition("DORMITORY_HYGIENE")
	if !reflect.DeepEqual(h.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"}) || h.TemplateStatus != "ACTIVE" {
		t.Fatal("dormitory hygiene import capability missing")
	}
	if !reflect.DeepEqual(m.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE", "IMPORT"}) || m.TemplateStatus != "ACTIVE" {
		t.Fatal("morning import capability missing")
	}
	for _, f := range m.Fields {
		if f.Key == "activityKind" {
			t.Fatal("morning activities must stay one type")
		}
	}
	for _, code := range []string{"DORMITORY_HYGIENE", "CLASSROOM_HYGIENE", "PUBLIC_DORMITORY_HYGIENE"} {
		d, _ := LookupInspectionDefinition(code)
		if code == "PUBLIC_DORMITORY_HYGIENE" && (!reflect.DeepEqual(d.AvailableActions, []string{"CREATE", "LIST", "EDIT", "SOFT_DELETE", "BATCH_DELETE"}) || d.TemplateStatus != "PLANNED") {
			t.Fatal("verified manual actions missing or unverified import enabled", code)
		}
		if d.Granularity != "OBJECT_INSPECTION" {
			t.Fatal(code)
		}
		var result FieldDefinition
		for _, f := range d.Fields {
			if f.Key == "result" {
				result = f
			}
		}
		if !reflect.DeepEqual(result.Enum, []string{"EXCELLENT", "QUALIFIED", "UNQUALIFIED"}) {
			t.Fatalf("result=%v", result.Enum)
		}
	}
}
func TestUnknownInspectionType(t *testing.T) {
	for _, code := range []string{"", "UNKNOWN", "REPORT"} {
		if _, err := LookupInspectionDefinition(code); err != ErrUnknownInspectionType {
			t.Fatalf("unknown %s: %v", code, err)
		}
	}
	if ErrUnknownInspectionType.Error() != "VALIDATION_FAILED" {
		t.Fatal("unstable error code")
	}
}
func TestInspectionCatalogReturnsIndependentCopies(t *testing.T) {
	a, _ := LookupInspectionDefinition("DORMITORY_HYGIENE")
	a.Fields[0].Label = "changed"
	a.DuplicateKey[0] = "changed"
	b, _ := LookupInspectionDefinition("DORMITORY_HYGIENE")
	if b.Fields[0].Label == "changed" || b.DuplicateKey[0] == "changed" {
		t.Fatal("catalog was mutated")
	}
}
