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
	return r
}
func classroomHygieneRow(n int) ParsedRow {
	r := hygieneRow(n)
	delete(r.Values, "dormitory_name")
	r.Values["classroom_name"] = "教学楼 101"
	return r
}
func classroomHygieneScope() AttendanceImportScope {
	s := hygieneScope()
	s.TypeCode = inspection.InspectionTypeClassroomHygiene
	return s
}
func TestClassroomHygieneAdapterUsesOneStudentAndRoomDayKey(t *testing.T) {
	repo := classroomHygieneRepository()
	row := classroomHygieneRow(5)
	result, err := NewClassroomHygieneAdapter(repo).Validate(context.Background(), classroomHygieneScope(), []ParsedRow{row})
	if err != nil || result.ValidCount != 1 {
		t.Fatal(result, err)
	}
	var details inspection.ClassroomHygieneDetails
	if err := json.Unmarshal(result.Rows[0].Record.Input.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details.ClassroomID != "c0000000-0000-0000-0000-000000000011" || details.AreaID != "" || len(details.ResponsibleStudentIDs) != 1 {
		t.Fatal(details)
	}
	duplicate := classroomHygieneRow(6)
	duplicate.Values["result"] = "合格"
	result, err = NewClassroomHygieneAdapter(repo).Validate(context.Background(), classroomHygieneScope(), []ParsedRow{row, duplicate})
	if err != nil || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatal(result, err)
	}
}
