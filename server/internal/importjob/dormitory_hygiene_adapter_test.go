package importjob

import (
	"context"
	"encoding/json"
	"four-check-management-system/server/internal/inspection"
	"testing"
)

type hygieneReferences struct {
	fakeAttendanceReferenceRepository
	objects   HygieneImportObjects
	objectErr error
}

func (r *hygieneReferences) LoadHygieneImportObjects(context.Context, HygieneImportLookup) (HygieneImportObjects, error) {
	return r.objects, r.objectErr
}
func hygieneRepository() *hygieneReferences {
	return &hygieneReferences{fakeAttendanceReferenceRepository: fakeAttendanceReferenceRepository{references: morningRefs()}, objects: HygieneImportObjects{Dormitories: map[string][]HygieneNamedReference{"示例区 示例楼 101": {{ID: "c0000000-0000-0000-0000-000000000003", Name: "示例区 示例楼 101"}}}}}
}
func hygieneScope() AttendanceImportScope {
	s := morningScope()
	s.TypeCode = inspection.InspectionTypeDormitoryHygiene
	return s
}
func hygieneRow(n int) ParsedRow {
	return ParsedRow{RowNo: n, Values: map[string]string{"dormitory_name": "示例区 示例楼 101", "inspection_date": "2026-10-01", "result": "优秀", "responsible_class_names": "示例班", "responsible_student_no": "000001"}}
}
func TestDormitoryHygieneAdapterKeepsOneResponsibleStudentAndRejectsRecheck(t *testing.T) {
	repo := hygieneRepository()
	adapter := NewDormitoryHygieneAdapter(repo)
	first, second := hygieneRow(5), hygieneRow(6)
	second.Values["responsible_student_no"] = "000001"
	second.Values["result"] = "合格"
	result, err := adapter.Validate(context.Background(), hygieneScope(), []ParsedRow{first, second})
	if err != nil || result.ValidCount != 1 || result.InvalidCount != 1 || !hasMorningError(result.Rows[1].Errors, ErrorDuplicateInBatch) {
		t.Fatal(result, err)
	}
	record := result.Rows[0].Record
	var details inspection.DormitoryHygieneDetails
	if err := json.Unmarshal(record.Input.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details.Result != inspection.HygieneExcellent || details.Time != "" || details.Round != "" || len(details.ResponsibleClassIDs) != 1 || len(details.ResponsibleStudentIDs) != 1 || len(record.References.Students) != 1 {
		t.Fatal(details, record.References)
	}
	repo.existing = map[string]struct{}{record.DedupKey: {}}
	result, err = adapter.Validate(context.Background(), hygieneScope(), []ParsedRow{first})
	if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatal(result, err)
	}
}
func TestDormitoryHygieneAdapterRequiresOneStudentAndDescriptionForUnqualified(t *testing.T) {
	for _, row := range []ParsedRow{
		func() ParsedRow { r := hygieneRow(5); r.Values["responsible_student_no"] = ""; return r }(),
		func() ParsedRow { r := hygieneRow(5); r.Values["responsible_student_no"] = "000001;000001"; return r }(),
		func() ParsedRow { r := hygieneRow(5); r.Values["result"] = "不合格"; return r }(),
	} {
		result, err := NewDormitoryHygieneAdapter(hygieneRepository()).Validate(context.Background(), hygieneScope(), []ParsedRow{row})
		if err != nil || result.ValidCount != 0 || result.InvalidCount != 1 {
			t.Fatal("invalid hygiene row accepted", result, err)
		}
	}
	row := hygieneRow(5)
	row.Values["result"], row.Values["description"] = "不合格", "门框积灰"
	result, err := NewDormitoryHygieneAdapter(hygieneRepository()).Validate(context.Background(), hygieneScope(), []ParsedRow{row})
	if err != nil || result.ValidCount != 1 {
		t.Fatal(result, err)
	}
}
