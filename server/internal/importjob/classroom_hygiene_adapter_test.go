package importjob

import (
	"context"
	"encoding/json"
	"four-check-management-system/server/internal/inspection"
	"testing"
)

func classroomHygieneRepository() *hygieneReferences {
	r := hygieneRepository()
	r.objects.Classrooms = map[string][]HygieneNamedReference{"教学楼 101": {{ID: "c0000000-0000-0000-0000-000000000011", Name: "教学楼 101"}}}
	r.objects.Areas = map[string][]HygieneNamedReference{"前排": {{ID: "c0000000-0000-0000-0000-000000000012", Name: "前排", ParentID: "c0000000-0000-0000-0000-000000000011"}, {ID: "c0000000-0000-0000-0000-000000000013", Name: "前排", ParentID: "c0000000-0000-0000-0000-000000000099"}}}
	return r
}
func classroomHygieneRow(n int) ParsedRow {
	r := hygieneRow(n)
	delete(r.Values, "dormitory_name")
	r.Values["classroom_name"] = "教学楼 101"
	r.Values["area_name"] = "前排"
	return r
}
func classroomHygieneScope() AttendanceImportScope {
	s := hygieneScope()
	s.TypeCode = inspection.InspectionTypeClassroomHygiene
	return s
}
func TestClassroomHygieneAdapterObjectScope(t *testing.T) {
	repo := classroomHygieneRepository()
	a := NewClassroomHygieneAdapter(repo)
	row := classroomHygieneRow(5)
	result, err := a.Validate(context.Background(), classroomHygieneScope(), []ParsedRow{row})
	if err != nil || result.ValidCount != 1 {
		t.Fatal(result, err)
	}
	var d inspection.ClassroomHygieneDetails
	if err := json.Unmarshal(result.Rows[0].Record.Input.Details, &d); err != nil {
		t.Fatal(err)
	}
	if d.ClassroomID != "c0000000-0000-0000-0000-000000000011" || d.AreaID != "c0000000-0000-0000-0000-000000000012" || len(d.ResponsibleStudentIDs) != 0 || len(d.ResponsibleClassIDs) != 1 {
		t.Fatal(d)
	}
	duplicate := classroomHygieneRow(6)
	duplicate.Values["result"] = "合格"
	duplicate.Values["responsible_student_nos"] = "000001"
	result, err = a.Validate(context.Background(), classroomHygieneScope(), []ParsedRow{row, duplicate})
	if err != nil || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatal(result, err)
	}
}
func TestClassroomHygieneAdapterRejectsInvalidReferences(t *testing.T) {
	for _, kind := range []string{"missing classroom", "other classroom area", "ambiguous classroom", "ambiguous area", "missing reason", "wrong grade"} {
		t.Run(kind, func(t *testing.T) {
			repo := classroomHygieneRepository()
			row := classroomHygieneRow(5)
			scope := classroomHygieneScope()
			switch kind {
			case "missing classroom":
				row.Values["classroom_name"] = "unknown"
			case "other classroom area":
				repo.objects.Areas["前排"] = repo.objects.Areas["前排"][1:]
			case "ambiguous classroom":
				repo.objects.Classrooms["教学楼 101"] = append(repo.objects.Classrooms["教学楼 101"], HygieneNamedReference{ID: "c0000000-0000-0000-0000-000000000019"})
			case "ambiguous area":
				repo.objects.Areas["前排"] = append(repo.objects.Areas["前排"], repo.objects.Areas["前排"][0])
			case "missing reason":
				row.Values["result"] = "不合格"
			case "wrong grade":
				scope.Grade = "其他年级"
			}
			result, err := NewClassroomHygieneAdapter(repo).Validate(context.Background(), scope, []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || result.InvalidCount != 1 {
				t.Fatal(result, err)
			}
		})
	}
}
