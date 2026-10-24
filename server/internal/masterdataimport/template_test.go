package masterdataimport

import (
	"testing"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
)

func TestSpecsCoverOnlyTheElevenApprovedMasterDataKinds(t *testing.T) {
	want := map[string][]string{
		masterdata.KindFaculty:           {"name"},
		masterdata.KindGrade:             {"year"},
		masterdata.KindMajor:             {"faculty_name", "name"},
		masterdata.KindStaff:             {"employee_no", "name", "gender", "phone", "faculty_name"},
		masterdata.KindClass:             {"name", "grade_year", "enrolled_size", "faculty_name", "major_name", "head_teacher_employee_no"},
		masterdata.KindDormitoryArea:     {"name"},
		masterdata.KindDormitoryBuilding: {"area_name", "name"},
		masterdata.KindDormitory:         {"area_name", "building_name", "room_no"},
		masterdata.KindTeachingBuilding:  {"name"},
		masterdata.KindClassroom:         {"building_name", "room_no"},
		masterdata.KindStudent:           {"student_no", "name", "grade_year", "faculty_name", "major_name", "class_name", "dormitory_area_name", "dormitory_building_name", "room_no", "gender", "enrollment_status"},
	}
	specs := Specs()
	if len(specs) != len(want) {
		t.Fatalf("spec count = %d, want %d", len(specs), len(want))
	}
	for kind, fields := range want {
		spec, ok := SpecFor(kind)
		if !ok {
			t.Fatalf("missing template for %q", kind)
		}
		if len(spec.Fields) != len(fields) {
			t.Fatalf("%s fields = %d, want %d", kind, len(spec.Fields), len(fields))
		}
		for index, field := range fields {
			if spec.Fields[index].Code != field {
				t.Fatalf("%s field %d = %q, want %q", kind, index, spec.Fields[index].Code, field)
			}
		}
	}
	if _, ok := SpecFor(masterdata.KindCourse); ok {
		t.Fatal("course import must remain out of scope")
	}
	if _, err := NewParser(masterdata.KindCourse, 16<<20); err == nil {
		t.Fatal("course parser unexpectedly available")
	}
}

func TestGeneratedTemplateMatchesItsParserContract(t *testing.T) {
	for _, spec := range Specs() {
		t.Run(spec.Kind, func(t *testing.T) {
			workbook, err := TemplateXLSX(spec.Kind)
			if err != nil {
				t.Fatal(err)
			}
			parser, err := NewParser(spec.Kind, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.Parse(spec.Kind+".xlsx", workbook)
			if err != nil {
				t.Fatalf("generated template failed to parse: %v", err)
			}
			if parsed.TemplateCode != spec.TemplateCode || parsed.TemplateVersion != spec.TemplateVersion || len(parsed.Fields) != len(spec.Fields) {
				t.Fatalf("parsed template contract = %+v", parsed)
			}
		})
	}
}

var _ *importjob.XLSXParser
