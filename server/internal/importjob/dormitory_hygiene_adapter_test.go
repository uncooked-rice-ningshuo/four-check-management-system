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
func (r *hygieneReferences) FindExistingHygieneKeys(ctx context.Context, s AttendanceImportScope, keys []string) (map[string]struct{}, error) {
	return r.FindExistingAttendanceKeys(ctx, s, keys)
}
func hygieneRepository() *hygieneReferences {
	return &hygieneReferences{fakeAttendanceReferenceRepository: fakeAttendanceReferenceRepository{references: morningRefs()}, objects: HygieneImportObjects{Dormitories: map[string][]HygieneNamedReference{"示例区 示例楼 101": {{ID: "c0000000-0000-0000-0000-000000000003", Name: "示例区 示例楼 101"}}}, Reasons: map[string][]HygieneNamedReference{"地面不洁": {{Code: "FLOOR", Name: "地面不洁"}}}}}
}
func hygieneScope() AttendanceImportScope {
	s := morningScope()
	s.TypeCode = inspection.InspectionTypeDormitoryHygiene
	return s
}
func hygieneRow(n int) ParsedRow {
	return ParsedRow{RowNo: n, Values: map[string]string{"dormitory_name": "示例区 示例楼 101", "inspection_date": "2026-10-01", "inspection_round": "第1轮", "result": "优秀", "responsible_class_names": "示例班"}}
}
func TestDormitoryHygieneAdapterKeepsObjectAndResponsibilities(t *testing.T) {
	repo := hygieneRepository()
	adapter := NewDormitoryHygieneAdapter(repo)
	first, second := hygieneRow(5), hygieneRow(6)
	second.Values["responsible_student_nos"] = "000001"
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
	if details.Result != inspection.HygieneExcellent || details.Round != "第1轮" || details.Time != "" || len(details.ResponsibleClassIDs) != 1 || len(details.ResponsibleStudentIDs) != 0 || len(record.References.Students) != 0 {
		t.Fatal(details, record.References)
	}
	repo.existing = map[string]struct{}{record.DedupKey: {}}
	result, err = adapter.Validate(context.Background(), hygieneScope(), []ParsedRow{first})
	if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorRecordAlreadyExists) {
		t.Fatal(result, err)
	}
}
func TestDormitoryHygieneAdapterRejectsInvalidObjectInput(t *testing.T) {
	for _, scenario := range []struct{ field, value string }{
		{"dormitory_name", "未知宿舍"}, {"responsible_class_names", "未知班级"}, {"responsible_student_nos", "unknown"}, {"responsible_class_names", "示例班;示例班"}, {"inspection_date", "2026-10-08"}, {"inspection_round", ""}, {"inspection_time", "09:00"}, {"result", "不合格"}, {"result", "其他"}, {"reason_name", "未知原因"},
	} {
		t.Run(scenario.field+scenario.value, func(t *testing.T) {
			row := hygieneRow(5)
			row.Values[scenario.field] = scenario.value
			result, err := NewDormitoryHygieneAdapter(hygieneRepository()).Validate(context.Background(), hygieneScope(), []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || result.InvalidCount != 1 {
				t.Fatal(result, err)
			}
		})
	}
	scope := hygieneScope()
	scope.Grade = "2025级"
	result, err := NewDormitoryHygieneAdapter(hygieneRepository()).Validate(context.Background(), scope, []ParsedRow{hygieneRow(5)})
	if err != nil || result.ValidCount != 0 {
		t.Fatal("out-of-scope responsibility", result, err)
	}
}

func TestDormitoryHygieneAdapterRejectsAmbiguousNames(t *testing.T) {
	for _, field := range []string{"dormitory_name", "reason_name"} {
		t.Run(field, func(t *testing.T) {
			repo := hygieneRepository()
			row := hygieneRow(5)
			if field == "dormitory_name" {
				repo.objects.Dormitories[row.Values[field]] = append(repo.objects.Dormitories[row.Values[field]], HygieneNamedReference{ID: "c0000000-0000-0000-0000-000000000099"})
			} else {
				row.Values["result"] = "不合格"
				row.Values[field] = "地面不洁"
				repo.objects.Reasons["地面不洁"] = append(repo.objects.Reasons["地面不洁"], HygieneNamedReference{Code: "OTHER"})
			}
			result, err := NewDormitoryHygieneAdapter(repo).Validate(context.Background(), hygieneScope(), []ParsedRow{row})
			if err != nil || result.ValidCount != 0 || !hasMorningError(result.Rows[0].Errors, ErrorReferenceMismatch) {
				t.Fatal("ambiguous name accepted", result, err)
			}
		})
	}
}

func TestDormitoryHygieneImportOtherReasonNeedsDescription(t *testing.T) {
	repo := hygieneRepository()
	repo.objects.Reasons["其他"] = []HygieneNamedReference{{Code: "OTHER", Name: "其他"}}
	row := hygieneRow(5)
	row.Values["result"] = "不合格"
	row.Values["reason_name"] = "其他"
	adapter := NewDormitoryHygieneAdapter(repo)
	result, err := adapter.Validate(context.Background(), hygieneScope(), []ParsedRow{row})
	if err != nil || result.ValidCount != 0 || result.InvalidCount != 1 {
		t.Fatal(result, err)
	}
	found := false
	for _, issue := range result.Rows[0].Errors {
		if issue.Field == "description" && issue.Message == "选择“其他”时必须填写具体原因" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing actionable description error", result.Rows[0].Errors)
	}
	row.Values["description"] = "门框积灰"
	result, err = adapter.Validate(context.Background(), hygieneScope(), []ParsedRow{row})
	if err != nil || result.ValidCount != 1 || result.InvalidCount != 0 {
		t.Fatal(result, err)
	}
}
