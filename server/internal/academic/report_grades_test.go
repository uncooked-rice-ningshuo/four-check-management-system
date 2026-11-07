package academic

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/audit"
)

type reportGradesStub struct{ calls int }

func (s *reportGradesStub) GetReportGrades(_ context.Context, id string) (ReportGradeScope, error) {
	return ReportGradeScope{SemesterID: id}, nil
}
func (s *reportGradesStub) ReplaceReportGrades(_ context.Context, scope ReportGradeScope, _ audit.Metadata) (ReportGradeScope, error) {
	s.calls++
	return scope, nil
}

func TestReportGradeScopeRejectsInvalidSelection(t *testing.T) {
	repository := &reportGradesStub{}
	service := NewReportGradeService(repository)
	semester := "10000000-0000-0000-0000-000000000001"
	grade := "10000000-0000-0000-0000-000000000002"
	for _, ids := range [][]string{nil, {grade, grade}, {"not-a-uuid"}} {
		_, err := service.Replace(context.Background(), ReportGradeScope{SemesterID: semester, GradeIDs: ids}, audit.Metadata{})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ids=%v: got %v, want invalid input", ids, err)
		}
	}
	if repository.calls != 0 {
		t.Fatalf("invalid selection reached repository %d times", repository.calls)
	}
	if _, err := service.Replace(context.Background(), ReportGradeScope{SemesterID: semester, GradeIDs: []string{grade}}, audit.Metadata{}); err != nil {
		t.Fatal(err)
	}
	if repository.calls != 1 {
		t.Fatal("valid selection was not saved")
	}
}
