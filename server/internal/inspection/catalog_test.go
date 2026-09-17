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
		if d.Code != "ATTENDANCE_RATE" && len(d.AvailableActions) != 0 {
			t.Fatalf("unimplemented actions enabled for %s", d.Code)
		}
	}
	if len(tables) != 6 {
		t.Fatalf("tables=%d", len(tables))
	}
	a, _ := LookupInspectionDefinition("ATTENDANCE_RATE")
	u, _ := LookupInspectionDefinition("UNATTENDED_CLASSROOM")
	if a.ExtensionTable != u.ExtensionTable || !reflect.DeepEqual(a.Fields, u.Fields) {
		t.Fatal("course absence schemas drifted")
	}
	m, _ := LookupInspectionDefinition("MORNING_ATTENDANCE")
	for _, f := range m.Fields {
		if f.Key == "activityKind" {
			t.Fatal("morning activities must stay one type")
		}
	}
	for _, code := range []string{"DORMITORY_HYGIENE", "CLASSROOM_HYGIENE", "PUBLIC_DORMITORY_HYGIENE"} {
		d, _ := LookupInspectionDefinition(code)
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
