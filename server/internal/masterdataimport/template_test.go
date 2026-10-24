package masterdataimport

import (
	"bytes"
	"strings"
	"testing"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
	"github.com/xuri/excelize/v2"
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
	studentSpec, _ := SpecFor(masterdata.KindStudent)
	fieldFormats := make(map[string]string, len(studentSpec.Fields))
	for _, field := range studentSpec.Fields {
		fieldFormats[field.Code] = field.Format
	}
	for _, field := range []string{"gender", "enrollment_status"} {
		if !strings.Contains(fieldFormats[field], "显示名称") || !strings.Contains(fieldFormats[field], "选项值") {
			t.Errorf("student %s format must explain both dictionary input forms: %q", field, fieldFormats[field])
		}
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

func TestGeneratedTemplateShowsFieldFormatsAndExamplesBeforeImportRows(t *testing.T) {
	examples := map[string][]string{
		masterdata.KindFaculty:           {"计算机科学与工程学院"},
		masterdata.KindGrade:             {"2026"},
		masterdata.KindMajor:             {"计算机科学与工程学院", "软件工程"},
		masterdata.KindStaff:             {"T2026001", "张老师", "男", "13800000000", "计算机科学与工程学院"},
		masterdata.KindClass:             {"2026级软件工程一班", "2026", "32", "计算机科学与工程学院", "软件工程", "T2026001"},
		masterdata.KindDormitoryArea:     {"4 区"},
		masterdata.KindDormitoryBuilding: {"4 区", "1栋"},
		masterdata.KindDormitory:         {"4 区", "1栋", "001"},
		masterdata.KindTeachingBuilding:  {"第一教学楼"},
		masterdata.KindClassroom:         {"第一教学楼", "101"},
		masterdata.KindStudent:           {"2000000000", "张同学", "2026", "计算机科学与工程学院", "软件工程", "2026级软件工程一班", "4 区", "1栋", "001", "男", "在籍"},
	}

	for _, spec := range Specs() {
		t.Run(spec.Kind, func(t *testing.T) {
			content, err := TemplateXLSX(spec.Kind)
			if err != nil {
				t.Fatal(err)
			}
			book, err := excelize.OpenReader(bytes.NewReader(content))
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()

			rows, err := book.GetRows(spec.SheetName)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) < 5 {
				t.Fatalf("template has %d rows, want format and example rows", len(rows))
			}
			formats := make([]string, len(spec.Fields))
			for index, field := range spec.Fields {
				formats[index] = field.Format
				if !hasExplicitDataType(field.Format) {
					t.Errorf("field %s format must state its data type: %q", field.Code, field.Format)
				}
			}
			if strings.Join(rows[3], "\x00") != strings.Join(formats, "\x00") {
				t.Fatalf("row 4 field formats = %q, want %q", rows[3], formats)
			}
			if strings.Join(rows[4], "\x00") != strings.Join(examples[spec.Kind], "\x00") {
				t.Fatalf("row 5 examples = %q, want %q", rows[4], examples[spec.Kind])
			}

			parser, err := NewParser(spec.Kind, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.Parse(spec.Kind+".xlsx", content)
			if err != nil {
				t.Fatalf("generated template failed to parse: %v", err)
			}
			if len(parsed.Rows) != 0 {
				t.Fatalf("example row was parsed as business data: %+v", parsed.Rows)
			}

			actualRow := make([]any, len(spec.Fields))
			for index := range actualRow {
				actualRow[index] = "真实数据"
			}
			if err := book.SetSheetRow(spec.SheetName, "A6", &actualRow); err != nil {
				t.Fatal(err)
			}
			updated, err := book.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err = parser.Parse(spec.Kind+".xlsx", updated.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.Rows) != 1 || parsed.Rows[0].RowNo != 6 {
				t.Fatalf("first business row = %+v, want row 6 only", parsed.Rows)
			}
		})
	}
}

func hasExplicitDataType(format string) bool {
	for _, dataType := range []string{"文本；", "数字；", "整数；", "枚举（字典）；"} {
		if strings.HasPrefix(format, dataType) {
			return true
		}
	}
	return false
}

var _ *importjob.XLSXParser
