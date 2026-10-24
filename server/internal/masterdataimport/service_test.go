package masterdataimport

import (
	"context"
	"testing"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/masterdata"
)

type memoryCatalog struct {
	records      map[string][]masterdata.Record
	dictionaries map[string][]string
}

func (m memoryCatalog) FindMasterDataImportRecords(_ context.Context, kind string, keys []LookupKey, includeDisabled bool) ([]masterdata.Record, error) {
	var result []masterdata.Record
	for _, record := range m.records[kind] {
		if !includeDisabled && !record.Enabled {
			continue
		}
		for _, key := range keys {
			if matchesKey(kind, []masterdata.Record{record}, key) {
				result = append(result, record)
				break
			}
		}
	}
	return result, nil
}

func (m memoryCatalog) EnabledDictionaryValues(_ context.Context, code string) ([]string, error) {
	return m.dictionaries[code], nil
}

func TestValidateRowsRejectsWithinFileAndExistingDuplicates(t *testing.T) {
	existing := masterdata.Record{ID: "disabled-faculty", Kind: masterdata.KindFaculty, Name: "已停用院系", Enabled: false}
	catalog := memoryCatalog{records: map[string][]masterdata.Record{masterdata.KindFaculty: {existing}}}
	service := NewService(nil, catalog, 16<<20)
	spec, _ := SpecFor(masterdata.KindFaculty)
	rows, err := service.validateRows(context.Background(), spec, []importjob.ParsedRow{
		{RowNo: 5, Values: map[string]string{"name": "重复院系"}},
		{RowNo: 6, Values: map[string]string{"name": " 重复院系 "}},
		{RowNo: 7, Values: map[string]string{"name": "已停用院系"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != RowInvalid || rows[1].Status != RowInvalid {
		t.Fatalf("in-file duplicates must both be blocked: %+v", rows)
	}
	if rows[2].Status != RowInvalid {
		t.Fatalf("disabled existing record must still block duplicate create: %+v", rows[2])
	}
	if rows[0].Record != nil || rows[1].Record != nil || rows[2].Record != nil {
		t.Fatal("invalid rows must not produce records eligible for insertion")
	}
}

func TestValidateStudentsResolvesNamesAndCanonicalizesIdentifiers(t *testing.T) {
	catalog := memoryCatalog{
		records: map[string][]masterdata.Record{
			masterdata.KindGrade:             {{ID: "grade-2026", Kind: masterdata.KindGrade, Name: "2026级", Enabled: true}},
			masterdata.KindFaculty:           {{ID: "faculty-1", Kind: masterdata.KindFaculty, Name: "计算机学院", Enabled: true}},
			masterdata.KindMajor:             {{ID: "major-1", Kind: masterdata.KindMajor, Name: "软件工程", FacultyID: "faculty-1", Enabled: true}},
			masterdata.KindClass:             {{ID: "class-1", Kind: masterdata.KindClass, Name: "软工2601", GradeID: "grade-2026", FacultyID: "faculty-1", MajorID: "major-1", Enabled: true}},
			masterdata.KindDormitoryArea:     {{ID: "area-1", Kind: masterdata.KindDormitoryArea, Name: "东区", Enabled: true}},
			masterdata.KindDormitoryBuilding: {{ID: "building-1", Kind: masterdata.KindDormitoryBuilding, Name: "1号楼", DormitoryAreaID: "area-1", Enabled: true}},
			masterdata.KindDormitory:         {{ID: "dorm-1", Kind: masterdata.KindDormitory, Name: "401", RoomNo: "401", DormitoryBuildingID: "building-1", Enabled: true}},
		},
		dictionaries: map[string][]string{"GENDER": {"MALE", "FEMALE", "UNKNOWN"}, "ENROLLMENT_STATUS": {"ENROLLED", "SUSPENDED", "GRADUATED", "LEFT"}},
	}
	service := NewService(nil, catalog, 16<<20)
	spec, _ := SpecFor(masterdata.KindStudent)
	rows, err := service.validateRows(context.Background(), spec, []importjob.ParsedRow{{RowNo: 5, Values: map[string]string{
		"student_no": "s001", "name": "学生甲", "grade_year": "2026", "faculty_name": "计算机学院", "major_name": "软件工程",
		"class_name": "软工2601", "dormitory_area_name": "东区", "dormitory_building_name": "1号楼", "room_no": "401",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != RowValid || rows[0].Record == nil {
		t.Fatalf("preview row = %+v", rows[0])
	}
	record := rows[0].Record
	if record.Code != "S001" || record.GradeID != "grade-2026" || record.FacultyID != "faculty-1" || record.MajorID != "major-1" || record.ClassID != "class-1" || record.DormitoryID != "dorm-1" {
		t.Fatalf("resolved record = %+v", record)
	}
	if record.Gender != "UNKNOWN" || record.EnrollmentStatus != "ENROLLED" || !record.Enabled || record.ID == "" {
		t.Fatalf("defaulted record = %+v", record)
	}
}

func TestValidateStudentRejectsClassFromAnotherGrade(t *testing.T) {
	catalog := memoryCatalog{
		records: map[string][]masterdata.Record{
			masterdata.KindGrade: {{ID: "grade-2026", Kind: masterdata.KindGrade, Name: "2026级", Enabled: true}},
			masterdata.KindClass: {{ID: "class-2025", Kind: masterdata.KindClass, Name: "软工2501", GradeID: "grade-2025", Enabled: true}},
		},
		dictionaries: map[string][]string{"GENDER": {"UNKNOWN"}, "ENROLLMENT_STATUS": {"ENROLLED"}},
	}
	service := NewService(nil, catalog, 16<<20)
	spec, _ := SpecFor(masterdata.KindStudent)
	rows, err := service.validateRows(context.Background(), spec, []importjob.ParsedRow{{RowNo: 5, Values: map[string]string{
		"student_no": "s001", "name": "学生甲", "grade_year": "2026", "class_name": "软工2501",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != RowInvalid || !hasRowError(rows[0], "class_name") {
		t.Fatalf("grade mismatch row = %+v", rows[0])
	}
}

func TestValidateClassEnrolledSizeRequiresUnsigned32BitInteger(t *testing.T) {
	catalog := memoryCatalog{records: map[string][]masterdata.Record{
		masterdata.KindGrade: {{ID: "grade-2026", Kind: masterdata.KindGrade, Name: "2026级", Enabled: true}},
	}}
	service := NewService(nil, catalog, 16<<20)
	spec, _ := SpecFor(masterdata.KindClass)
	for _, test := range []struct {
		value string
		valid bool
	}{
		{value: "0", valid: true},
		{value: "2147483647", valid: true},
		{value: "-0"},
		{value: "+1"},
		{value: "1.5"},
		{value: "2,000"},
		{value: "2147483648"},
	} {
		rows, err := service.validateRows(context.Background(), spec, []importjob.ParsedRow{{RowNo: 5, Values: map[string]string{
			"name": "软工2601", "grade_year": "2026", "enrolled_size": test.value,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		if gotValid := rows[0].Status == RowValid; gotValid != test.valid {
			t.Fatalf("enrolled_size %q valid=%v want %v; errors=%+v", test.value, gotValid, test.valid, rows[0].Errors)
		}
	}
}

func hasRowError(row PreviewRow, field string) bool {
	for _, issue := range row.Errors {
		if issue.Field == field {
			return true
		}
	}
	return false
}
