package inspection

import (
	"context"
	"errors"
	"testing"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

type manualFixture struct {
	saved   int
	record  AttendanceRecord
	rules   permission.EffectiveResult
	failure error
}

func (f *manualFixture) SaveManualAttendance(_ context.Context, r AttendanceRecord, m audit.Mutation) (AttendanceRecord, error) {
	f.saved++
	if m.Metadata.ActorID != r.CreatedBy || m.Entry.ResourceID != r.ID {
		return AttendanceRecord{}, errors.New("incorrect audit")
	}
	return r, f.failure
}
func (f *manualFixture) ValidateManualAttendance(context.Context, ManualAttendanceInput, academic.Week, string) (AttendanceRecord, error) {
	return f.record, nil
}
func (f *manualFixture) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return f.rules, nil
}
func (f *manualFixture) List(context.Context) (academic.Calendar, error) {
	return academic.Calendar{AcademicYears: []academic.AcademicYear{{ID: "year", Enabled: true}}, Semesters: []academic.Semester{{ID: "semester", AcademicYearID: "year", Enabled: true}}, Weeks: []academic.Week{{ID: "week", SemesterID: "semester", Enabled: true, StartDate: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)}}}, nil
}
func (f *manualFixture) ListCatalogs(context.Context, bool) (organization.Catalogs, error) {
	return organization.Catalogs{Departments: []organization.Department{{ID: "department", Enabled: true}}, InspectionTypes: []organization.InspectionType{{ID: "type", Code: InspectionTypeAttendance, Enabled: true}}}, nil
}
func (f *manualFixture) ListScopes(context.Context) ([]organization.DepartmentInspectionScope, error) {
	return []organization.DepartmentInspectionScope{{DepartmentID: "department", InspectionTypeID: "type", Enabled: true, CanEnter: true}}, nil
}
func TestManualAttendanceCreatesAuditedDraftAndRejectsDeniedScope(t *testing.T) {
	f := &manualFixture{record: AttendanceRecord{ID: "record", ClassID: "class", GradeSnapshot: "2024", Source: RecordSourceExcel}}
	s := NewManualService(f, f, f, f, f)
	input := ManualAttendanceInput{SemesterID: "semester", WeekID: "week", DepartmentID: "department", InspectionTypeID: "type", ClassID: "class", Grade: "2024"}
	actor := ManualActor{ID: "actor", IsSuperAdmin: true}
	got, err := s.Create(context.Background(), actor, input, audit.Metadata{})
	if err != nil || got.Source != RecordSourceManual || got.Status != RecordStatusDraft || got.CreatedBy != "actor" || f.saved != 1 {
		t.Fatalf("record=%+v err=%v saved=%d", got, err, f.saved)
	}
	actor.IsSuperAdmin = false
	if _, err = s.Create(context.Background(), actor, input, audit.Metadata{}); !errors.Is(err, ErrScopeMismatch) || f.saved != 1 {
		t.Fatalf("denied err=%v saved=%d", err, f.saved)
	}
	f.rules = permission.EffectiveResult{Permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: "class"}}}}
	if _, err = s.Create(context.Background(), actor, input, audit.Metadata{}); err != nil {
		t.Fatal(err)
	}
	f.record.GradeSnapshot = "2025"
	f.rules.Permissions = append(f.rules.Permissions, permission.EffectivePermission{Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: permission.Scope{Grade: "2025"}})
	if _, err = s.Create(context.Background(), actor, input, audit.Metadata{}); !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("actual grade deny=%v", err)
	}
}
